package server

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
)

// sessionKeyBytes maps the named keys to the terminal sequences both
// harnesses read. Names are matched without case.
var sessionKeyBytes = map[string]string{
	"esc":       "\x1b",
	"enter":     "\r",
	"tab":       "\t",
	"shift+tab": "\x1b[Z",
	"backspace": "\x7f",
	"up":        "\x1b[A",
	"down":      "\x1b[B",
	"right":     "\x1b[C",
	"left":      "\x1b[D",
	"ctrl+c":    "\x03",
}

func (s *Server) sessionKeysWrite(w http.ResponseWriter, r *http.Request) {
	s.sessionKeysHandler(promptSourceCLI)(w, r)
}

// sessionKeysWeb is sessionKeysWrite for the web mux.
func (s *Server) sessionKeysWeb() http.HandlerFunc { return s.sessionKeysHandler(promptSourceWeb) }

func (s *Server) sessionKeysHandler(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.SessionKeysRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := s.keysSession(req, source)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}

// keysSession sends a person's key or text to a live session. It stops gx's
// own nudges, because the holder counts it as a person's input.
func (s *Server) keysSession(req wire.SessionKeysRequest, source string) (wire.SessionResult, error) {
	seq, n, refusal := validateSessionKeys(req)
	if strings.HasPrefix(req.ID, queue.AgentIDPrefix) {
		return sessionRefusal(wire.ReasonSessionReadOnly, fmt.Sprintf("%s is a Queue agent; its loop owns it", req.ID)), nil
	}
	if req.Permission != "" && refusal == nil {
		return s.answerSessionPermission(req, source)
	}
	dir, res, err := s.sendSessionKeys(req, source, seq, n, refusal)
	if res.Refused || err != nil || !req.OnlyBlocked {
		return res, err
	}
	return s.waitLeftBlocked(req.ID, dir)
}

// sendSessionKeys checks the request against the session and sends it, under
// metaMu so the guard and the send see one status.
func (s *Server) sendSessionKeys(req wire.SessionKeysRequest, source, seq string, n int, refusal *wire.SessionResult) (string, wire.SessionResult, error) {
	s.sessions.watches.metaMu.Lock()
	defer s.sessions.watches.metaMu.Unlock()
	dir, meta, res, err := s.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return dir, res, err
	}
	if refusal != nil {
		return dir, *refusal, nil
	}
	st := meta.info().Status
	switch {
	case !chat.AcceptsKeys(st.State):
		return dir, sessionRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s is %s", req.ID, st.State)), nil
	case req.OnlyBlocked && st.State != wire.SessionBlocked:
		return dir, sessionRefusal(wire.ReasonNotBlocked, fmt.Sprintf("session %s is %s, not blocked", req.ID, st.State)), nil
	}
	c, err := nativerunner.DialHolder(dir)
	if err != nil {
		return dir, sessionRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has no live holder", req.ID)), nil
	}
	if req.Key != "" && req.OnlyBlocked {
		// A guarded key (Close dialog) is no one typing into the session: a person's keys would wake
		// its stream and stop a design session's countdown.
		err = c.SendKeys(seq)
	} else if req.Key != "" {
		err = c.SendPersonKeys(seq)
	} else {
		err = c.TypePersonTextBy(seq, false)
	}
	if err != nil {
		return dir, wire.SessionResult{}, fmt.Errorf("send keys to session %s: %w", req.ID, err)
	}
	action := "text"
	if req.Key != "" {
		action = "key"
	}
	ev := ralphloop.Event{
		Time: time.Now(), Type: string(events.SessionKeyed),
		Session: req.ID, Source: source, Action: action, Bytes: n,
	}
	if err := ralphloop.AppendServerEvent(s.cfg.TicketStore, ev); err != nil {
		s.log.Warn("logging session keys", "session", req.ID, "err", err)
	}
	if meta.Design && !req.OnlyBlocked {
		if err := s.notePersonTyped(dir, meta, "Auto-advance stopped: you pressed a key"); err != nil {
			return dir, wire.SessionResult{}, err
		}
	}
	info := meta.info()
	return dir, wire.SessionResult{Session: &info}, nil
}

// answerSessionPermission answers the permission dialog of a Blocked session.
// It is guarded, so it leaves a Design session's countdown alone.
func (s *Server) answerSessionPermission(req wire.SessionKeysRequest, source string) (wire.SessionResult, error) {
	s.sessions.watches.metaMu.Lock()
	dir, meta, res, err := s.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		s.sessions.watches.metaMu.Unlock()
		return res, err
	}
	st := meta.info().Status
	switch {
	case !chat.AcceptsKeys(st.State):
		s.sessions.watches.metaMu.Unlock()
		return sessionRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s is %s", req.ID, st.State)), nil
	case st.State != wire.SessionBlocked || st.Kind != "permission":
		s.sessions.watches.metaMu.Unlock()
		return sessionRefusal(wire.ReasonNotBlocked, fmt.Sprintf("session %s is not blocked on a permission", req.ID)), nil
	}
	c, err := nativerunner.DialHolder(dir)
	// The answer waits for the agent to move on; the status update that ends the
	// wait needs the lock.
	s.sessions.watches.metaMu.Unlock()
	if err != nil {
		return sessionRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has no live holder", req.ID)), nil
	}
	grace := cmp.Or(s.cfg.PromptGrace, claudeui.DefaultNudgeCaps.Grace)
	answer := agentrunner.Answer{Decision: agentrunner.Decision(req.Permission), RequestID: req.RequestID}
	if err := nativerunner.AnswerSessionPermission(c, dir, agentrunner.Kind(meta.Harness), answer, 20*time.Millisecond, grace); err != nil {
		return sessionAnswerRefusal(req.ID, err)
	}
	ev := ralphloop.Event{
		Time: time.Now(), Type: string(events.SessionKeyed),
		Session: req.ID, Source: source, Action: "permission", Bytes: 1,
	}
	if err := ralphloop.AppendServerEvent(s.cfg.TicketStore, ev); err != nil {
		s.log.Warn("logging session permission", "session", req.ID, "err", err)
	}
	info := meta.info()
	return wire.SessionResult{Session: &info}, nil
}

// sessionAnswerRefusal maps a permission answer's error to its refusal; others
// are errors.
func sessionAnswerRefusal(id string, err error) (wire.SessionResult, error) {
	switch {
	case errors.Is(err, agentrunner.ErrRequestMismatch):
		return sessionRefusal(wire.ReasonRequestChanged, err.Error()), nil
	case errors.Is(err, nativerunner.ErrNotApprovable), errors.Is(err, agentrunner.ErrInvalidDecision):
		return sessionRefusal(wire.ReasonNotApprovable, err.Error()), nil
	case errors.Is(err, agentrunner.ErrNotDelivered):
		return sessionRefusal(wire.ReasonKeysNotDelivered, err.Error()), nil
	case errors.Is(err, agentrunner.ErrNotReady):
		return sessionRefusal(wire.ReasonNotBlocked, fmt.Sprintf("session %s is not blocked on a permission", id)), nil
	case errors.Is(err, agentrunner.ErrNotFound), errors.Is(err, agentrunner.ErrSessionDied):
		return sessionRefusal(wire.ReasonSessionNotLive, err.Error()), nil
	}
	return wire.SessionResult{}, fmt.Errorf("answer permission of session %s: %w", id, err)
}

// waitLeftBlocked waits up to the prompt grace for a guarded send to move the
// session out of Blocked.
func (s *Server) waitLeftBlocked(id, dir string) (wire.SessionResult, error) {
	grace := cmp.Or(s.cfg.PromptGrace, claudeui.DefaultNudgeCaps.Grace)
	deadline := time.Now().Add(grace)
	for {
		s.sessions.watches.metaMu.Lock()
		meta, err := readSessionMeta(dir)
		s.sessions.watches.metaMu.Unlock()
		if err != nil {
			return wire.SessionResult{}, err
		}
		if meta.info().Status.State != wire.SessionBlocked {
			info := meta.info()
			return wire.SessionResult{Session: &info}, nil
		}
		if time.Now().After(deadline) {
			return sessionRefusal(wire.ReasonKeysNotDelivered, fmt.Sprintf("session %s is still blocked", id)), nil
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// validateSessionKeys checks the request's shape. It returns the bytes to send
// and how many keys or bytes the event counts, or a refusal.
func validateSessionKeys(req wire.SessionKeysRequest) (string, int, *wire.SessionResult) {
	bad := func(reason, msg string) (string, int, *wire.SessionResult) {
		res := sessionRefusal(reason, msg)
		return "", 0, &res
	}
	inputs := 0
	for _, set := range []bool{req.Key != "", req.Text != "", req.Permission != ""} {
		if set {
			inputs++
		}
	}
	switch {
	case inputs != 1:
		return bad(wire.ReasonSessionBadRequest, "want exactly one of key, text or permission")
	case req.Permission != "":
		if req.Permission != string(agentrunner.DecisionAllow) && req.Permission != string(agentrunner.DecisionDeny) {
			return bad(wire.ReasonSessionBadRequest, fmt.Sprintf("permission is %q, want allow or deny", req.Permission))
		}
		return "", 1, nil
	case req.Key != "":
		seq, ok := sessionKeyBytes[strings.ToLower(req.Key)]
		if !ok {
			return bad(wire.ReasonSessionBadRequest, fmt.Sprintf("unknown key %q", req.Key))
		}
		return seq, 1, nil
	case strings.IndexFunc(req.Text, func(r rune) bool { return r < ' ' || r == 0x7f }) >= 0:
		return bad(wire.ReasonSessionBadRequest, "text may not hold a control character")
	case strings.HasPrefix(req.Text, "!"):
		return bad(wire.ReasonPromptPrefixNotAllowed, "text may not start with !")
	case len(req.Text) > maxSessionPromptBytes:
		return bad(wire.ReasonPromptTextTooLong, fmt.Sprintf("text is over %d bytes", maxSessionPromptBytes))
	case !utf8.ValidString(req.Text):
		return bad(wire.ReasonSessionBadRequest, "text is not valid UTF-8")
	}
	return req.Text, len(req.Text), nil
}
