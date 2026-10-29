package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
)

// Refusal reasons of the session prompt route, beside the session ones.
const (
	ReasonPromptHarnessNotSupported = "harness-not-supported"
	ReasonPromptPrefixNotAllowed    = "prefix-not-allowed"
	ReasonPromptTextTooLong         = "text-too-long"
)

// maxSessionPromptBytes caps the text one send types.
const maxSessionPromptBytes = 32 * 1024

// validatePromptText normalizes text typed into a session and refuses a
// slash or bang prefix and an oversized text. Empty text passes: each route
// decides whether that is allowed. what names the text in refusal messages.
func validatePromptText(text, what string) (string, *SessionResult) {
	text = strings.TrimSpace(normalizeAnswer(text))
	switch {
	case strings.HasPrefix(text, "/") || strings.HasPrefix(text, "!"):
		res := sessionRefusal(ReasonPromptPrefixNotAllowed, what+" may not start with / or !")
		return text, &res
	case len(text) > maxSessionPromptBytes:
		res := sessionRefusal(ReasonPromptTextTooLong, fmt.Sprintf("%s is over %d bytes", what, maxSessionPromptBytes))
		return text, &res
	}
	return text, nil
}

// Sources of a session prompt, as the event's source.
const (
	promptSourceWeb = "web"
	promptSourceCLI = "cli"
)

// SessionPromptRequest is the body of POST /v1/sessions/prompt. LastMessage is
// the id of the last chat message the page saw; the web mux refuses a send once
// the transcript has moved past it.
type SessionPromptRequest struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	LastMessage string `json:"last_message,omitempty"`
	// Continue replaces Text: the server types /clear and /gx-design <epic>.
	Continue bool `json:"continue,omitempty"`
}

func (s *Server) sessionPromptWrite(w http.ResponseWriter, r *http.Request) {
	s.sessionPromptHandler(promptSourceCLI)(w, r)
}

// sessionPromptWeb is sessionPromptWrite for the web mux.
func (s *Server) sessionPromptWeb() http.HandlerFunc { return s.sessionPromptHandler(promptSourceWeb) }

func (s *Server) sessionPromptHandler(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SessionPromptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := s.promptSession(req, source)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}

// promptSession types a person's text into an Idle Claude session, in the
// background so the nudges outlive the request. Continue is for design
// sessions only: it types /clear and /gx-design. It skips the draft guard:
// gx.follow-ups/07 tracks that.
func (s *Server) promptSession(req SessionPromptRequest, source string) (SessionResult, error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if req.Continue && !meta.Design {
		return sessionRefusal(ReasonSessionNotDesign, fmt.Sprintf("session %s is not a design session", req.ID)), nil
	}
	if meta.Harness != s.settings.Load().chatHarness() {
		return sessionRefusal(ReasonPromptHarnessNotSupported, fmt.Sprintf("session %s runs a harness with no chat", req.ID)), nil
	}
	text, refusal := validatePromptText(req.Text, "text")
	if !req.Continue {
		if refusal != nil {
			return *refusal, nil
		}
		if text == "" {
			return sessionRefusal(ReasonEmptyPrompt, "text to type is required"), nil
		}
	}
	if source == promptSourceWeb {
		last, err := s.lastChatMessageID(req.ID, meta)
		if err != nil {
			return SessionResult{}, err
		}
		if last != req.LastMessage {
			return sessionRefusal(ReasonStale, "the conversation moved; review it first"), nil
		}
	}
	if st := meta.info().Status; !st.Promptable() {
		return sessionRefusal(ReasonSessionNotIdle, fmt.Sprintf("session %s is %s, not idle", req.ID, st.State)), nil
	}
	ev := ralphloop.Event{
		Time: time.Now(), Type: string(events.SessionPrompted),
		Session: req.ID, Source: source, Action: "text", Bytes: len(text),
	}
	if req.Continue {
		// The server builds the slash commands, so the prefix rule stays closed
		// to the page.
		action := designContinueAction(meta.Ticket, meta.Harness)
		s.cancelAutoAdvance(req.ID, "Auto-advance stopped: you continued")
		meta.Actionable = SessionActionable{}
		meta.AutoSends = 0
		if err := writeSessionMeta(dir, meta); err != nil {
			return SessionResult{}, err
		}
		s.events.publish(EventSessionChanged, req.ID)
		s.sessionWatches.start(func() {
			s.sendAction(dir, "Continue", action)
		})
		ev.Action = "continue"
		for _, a := range action {
			ev.Bytes += len(a)
		}
	} else {
		s.sessionWatches.start(func() { s.sendPrompt(dir, text) })
		stopped := s.cancelAutoAdvance(req.ID, "Auto-advance stopped: you typed")
		changed := meta.AutoSends != 0
		meta.AutoSends = 0
		if !stopped && !meta.Actionable.GxRaised && meta.Actionable.Reason != "" {
			meta.Actionable = SessionActionable{}
			changed = true
		}
		if changed {
			if err := writeSessionMeta(dir, meta); err != nil {
				return SessionResult{}, err
			}
			s.events.publish(EventSessionChanged, req.ID)
		}
	}
	if err := ralphloop.AppendServerEvent(s.cfg.TicketStore, ev); err != nil {
		s.log.Warn("logging session prompt", "session", req.ID, "err", err)
	}
	info := meta.info()
	return SessionResult{Session: &info}, nil
}

// lastChatMessageID is the id of the newest visible message of the session's
// chat, or "" with none.
func (s *Server) lastChatMessageID(id string, meta sessionMeta) (string, error) {
	path, ok := s.claudeTranscript(id, meta)
	if !ok {
		return "", nil
	}
	final := s.designFinal(meta.Ticket)
	chat, err := s.chats.read(path, final, chatLoader(meta.Ticket, final))
	if os.IsNotExist(err) || len(chat.msgs) == 0 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return chat.msgs[len(chat.msgs)-1].ID, nil
}
