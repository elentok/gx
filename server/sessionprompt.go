package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
)

// Sources of a session prompt, as the event's source.
const (
	promptSourceWeb = "web"
	promptSourceCLI = "cli"
)

func (s *Server) sessionPromptWrite(w http.ResponseWriter, r *http.Request) {
	s.sessionPromptHandler(promptSourceCLI)(w, r)
}

// sessionPromptWeb is sessionPromptWrite for the web mux.
func (s *Server) sessionPromptWeb() http.HandlerFunc { return s.sessionPromptHandler(promptSourceWeb) }

func (s *Server) sessionPromptHandler(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.SessionPromptRequest
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
func (s *Server) promptSession(req wire.SessionPromptRequest, source string) (wire.SessionResult, error) {
	s.sessions.watches.metaMu.Lock()
	defer s.sessions.watches.metaMu.Unlock()
	dir, meta, res, err := s.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if req.Continue && !meta.Design {
		return sessionRefusal(wire.ReasonSessionNotDesign, fmt.Sprintf("session %s is not a design session", req.ID)), nil
	}
	if !s.settings.Load().HasChatReader(meta.Harness) {
		return sessionRefusal(wire.ReasonPromptHarnessNotSupported, fmt.Sprintf("session %s runs a harness with no chat", req.ID)), nil
	}
	text, refusal := wire.ValidatePromptText(req.Text, "text")
	if !req.Continue {
		if refusal != nil {
			return *refusal, nil
		}
		if text == "" {
			return sessionRefusal(wire.ReasonEmptyPrompt, "text to type is required"), nil
		}
	}
	if source == promptSourceWeb {
		last, err := s.lastChatMessageID(req.ID, meta)
		if err != nil {
			return wire.SessionResult{}, err
		}
		if last != req.LastMessage {
			return sessionRefusal(wire.ReasonStale, "the conversation moved; review it first"), nil
		}
	}
	if st := meta.info().Status; !st.Promptable() {
		return sessionRefusal(wire.ReasonSessionNotIdle, fmt.Sprintf("session %s is %s, not idle", req.ID, st.State)), nil
	}
	ev := ralphloop.Event{
		Time: time.Now(), Type: string(events.SessionPrompted),
		Session: req.ID, Source: source, Action: "text", Bytes: len(text),
	}
	if req.Continue {
		// The server builds the slash commands, so the prefix rule stays closed
		// to the page.
		action := chat.DesignContinueAction(meta.Ticket, meta.Harness)
		if res, refused := s.resumer.handoffRefusal(meta, action); refused {
			return res, nil
		}
		s.cancelAutoAdvance(req.ID, "Auto-advance stopped: you continued")
		meta.Actionable = wire.SessionActionable{}
		meta.AutoSends = 0
		if err := writeSessionMeta(dir, meta); err != nil {
			return wire.SessionResult{}, err
		}
		s.events.Publish(wire.EventSessionChanged, req.ID)
		s.resumer.startContinue(dir, meta, "Continue", action)
		ev.Action = "continue"
		for _, a := range action {
			ev.Bytes += len(a)
		}
	} else {
		s.sessions.watches.start(func() { s.sendPrompt(dir, text) })
		if name, ok := renameCommandArg(text); ok && name != meta.Name {
			// The harness renames itself, but a typed name never follows the
			// terminal title, so the Agents tab would keep the old one.
			meta.Name, meta.AutoName = name, false
			if err := writeSessionMeta(dir, meta); err != nil {
				return wire.SessionResult{}, err
			}
			s.events.Publish(wire.EventSessionChanged, req.ID)
		}
		if err := s.notePersonTyped(dir, meta, "Auto-advance stopped: you typed"); err != nil {
			return wire.SessionResult{}, err
		}
	}
	if err := ralphloop.AppendServerEvent(s.cfg.TicketStore, ev); err != nil {
		s.log.Warn("logging session prompt", "session", req.ID, "err", err)
	}
	info := meta.info()
	return wire.SessionResult{Session: &info}, nil
}

// notePersonTyped records a person's input on a session: it stops a running
// auto-advance countdown, resets the auto-send count and clears a derived
// Continue. The caller holds metaMu.
func (s *Server) notePersonTyped(dir string, meta sessionMeta, why string) error {
	stopped := s.cancelAutoAdvance(meta.ID, why)
	changed := meta.AutoSends != 0
	meta.AutoSends = 0
	if !stopped && !meta.Actionable.GxRaised && meta.Actionable.Reason != "" {
		meta.Actionable = wire.SessionActionable{}
		changed = true
	}
	if !changed {
		return nil
	}
	if err := writeSessionMeta(dir, meta); err != nil {
		return err
	}
	s.events.Publish(wire.EventSessionChanged, meta.ID)
	return nil
}

// lastChatMessageID is the id of the newest visible message of the session's
// chat, or "" with none.
func (s *Server) lastChatMessageID(id string, meta sessionMeta) (string, error) {
	path, kind, ok := s.sessionChat.Transcript(id, chatMeta(meta))
	if !ok {
		return "", nil
	}
	snap, err := s.sessionChat.ReadSnapshot(path, kind, meta.Ticket)
	if os.IsNotExist(err) || len(snap.Msgs) == 0 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return snap.Msgs[len(snap.Msgs)-1].ID, nil
}

// renameCommandArg is the new name in a "/rename <name>" or "/name <name>"
// prompt. A bare command (which asks the harness to pick a name) has none.
func renameCommandArg(text string) (string, bool) {
	cmd, arg, _ := strings.Cut(strings.TrimSpace(text), " ")
	if cmd != "/rename" && cmd != "/name" {
		return "", false
	}
	arg = strings.TrimSpace(arg)
	return arg, arg != "" && !strings.ContainsAny(arg, "\r\n")
}
