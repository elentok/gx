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
	ReasonHarnessNotSupported = "harness-not-supported"
	ReasonPrefixNotAllowed    = "prefix-not-allowed"
	ReasonTextTooLong         = "text-too-long"
)

// maxSessionPromptBytes caps the text one send types.
const maxSessionPromptBytes = 32 * 1024

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

// promptSession types a person's text into an Idle design session. It skips
// the draft guard: gx.follow-ups/07 tracks that.
func (s *Server) promptSession(req SessionPromptRequest, source string) (SessionResult, error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if !meta.Design {
		return sessionRefusal(ReasonNotDesignSession, fmt.Sprintf("session %s is not a design session", req.ID)), nil
	}
	if meta.Harness != s.settings.Load().chatHarness() {
		return sessionRefusal(ReasonHarnessNotSupported, fmt.Sprintf("session %s runs a harness with no chat", req.ID)), nil
	}
	text := strings.TrimSpace(normalizeAnswer(req.Text))
	if !req.Continue {
		switch {
		case text == "":
			return sessionRefusal(ReasonEmptyPrompt, "text to type is required"), nil
		case strings.HasPrefix(text, "/") || strings.HasPrefix(text, "!"):
			return sessionRefusal(ReasonPrefixNotAllowed, "text may not start with / or !"), nil
		case len(text) > maxSessionPromptBytes:
			return sessionRefusal(ReasonTextTooLong, fmt.Sprintf("text is over %d bytes", maxSessionPromptBytes)), nil
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
	if st := meta.info().Status.State; st != SessionIdle {
		return sessionRefusal(ReasonSessionNotIdle, fmt.Sprintf("session %s is %s, not idle", req.ID, st)), nil
	}
	ev := ralphloop.Event{
		Time: time.Now(), Type: string(events.SessionPrompted),
		Session: req.ID, Source: source, Action: "text", Bytes: len(text),
	}
	if req.Continue {
		// The server builds the slash commands, so the prefix rule stays closed
		// to the page.
		action := designContinueAction(meta.Ticket)
		meta.Actionable = SessionActionable{}
		if err := writeSessionMeta(dir, meta); err != nil {
			return SessionResult{}, err
		}
		s.events.publish(EventSessionChanged, req.ID)
		s.sessionWatches.start(func() {
			if err := s.sendAction(dir, action); err != nil {
				s.log.Info("continue stopped", "session", req.ID, "err", err)
			}
		})
		ev.Action, ev.Bytes = "continue", 0
	} else if err := typeSessionPrompt(dir, text); err != nil {
		return SessionResult{}, err
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
	path, ok := s.claudeTranscript(id, meta.ConversationID)
	if !ok {
		return "", nil
	}
	chat, err := s.chats.read(path, chatLoader(meta.Ticket))
	if os.IsNotExist(err) || len(chat.msgs) == 0 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return chat.msgs[len(chat.msgs)-1].ID, nil
}
