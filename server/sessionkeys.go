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

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
)

// ReasonSessionReadOnly refuses input to a Queue agent: its loop owns it.
const ReasonSessionReadOnly = "session-read-only"

// ReasonKeysNotDelivered: a guarded send left the session Blocked.
const ReasonKeysNotDelivered = "not-delivered"

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

// SessionKeysRequest is the body of POST /v1/sessions/keys. Exactly one input
// is set: Key (a named key) or Text (typed without Enter). OnlyBlocked sends
// only while the session is Blocked, and waits for it to leave Blocked.
type SessionKeysRequest struct {
	ID          string `json:"id"`
	Key         string `json:"key,omitempty"`
	Text        string `json:"text,omitempty"`
	OnlyBlocked bool   `json:"only_blocked,omitempty"`
	// Permission ("allow" or "deny") answers the permission dialog the session
	// is Blocked on, by the answer logic the Queue uses. RequestID, when set,
	// must be the dialog on screen.
	Permission string `json:"permission,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
}

func (s *Server) sessionKeysWrite(w http.ResponseWriter, r *http.Request) {
	s.sessionKeysHandler(promptSourceCLI)(w, r)
}

// sessionKeysWeb is sessionKeysWrite for the web mux.
func (s *Server) sessionKeysWeb() http.HandlerFunc { return s.sessionKeysHandler(promptSourceWeb) }

func (s *Server) sessionKeysHandler(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SessionKeysRequest
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
func (s *Server) keysSession(req SessionKeysRequest, source string) (SessionResult, error) {
	seq, n, refusal := validateSessionKeys(req)
	if strings.HasPrefix(req.ID, queueAgentIDPrefix) {
		return sessionRefusal(ReasonSessionReadOnly, fmt.Sprintf("%s is a Queue agent; its loop owns it", req.ID)), nil
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
func (s *Server) sendSessionKeys(req SessionKeysRequest, source, seq string, n int, refusal *SessionResult) (string, SessionResult, error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return dir, res, err
	}
	if refusal != nil {
		return dir, *refusal, nil
	}
	st := meta.info().Status
	switch {
	case !acceptsKeys(st.State):
		return dir, sessionRefusal(ReasonSessionNotLive, fmt.Sprintf("session %s is %s", req.ID, st.State)), nil
	case req.OnlyBlocked && st.State != SessionBlocked:
		return dir, sessionRefusal(ReasonNotBlocked, fmt.Sprintf("session %s is %s, not blocked", req.ID, st.State)), nil
	}
	c, err := nativerunner.DialHolder(dir)
	if err != nil {
		return dir, sessionRefusal(ReasonSessionNotLive, fmt.Sprintf("session %s has no live holder", req.ID)), nil
	}
	if req.Key != "" {
		err = c.SendPersonKeys(seq)
	} else {
		err = c.TypePersonTextBy(seq, false)
	}
	if err != nil {
		return dir, SessionResult{}, fmt.Errorf("send keys to session %s: %w", req.ID, err)
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
			return dir, SessionResult{}, err
		}
	}
	info := meta.info()
	return dir, SessionResult{Session: &info}, nil
}

func acceptsKeys(state string) bool {
	return state == SessionWorking || state == SessionIdle || state == SessionBlocked || state == SessionRateLimited
}

// answerSessionPermission answers the permission dialog of a Blocked session.
// It is guarded, so it leaves a Design session's countdown alone.
func (s *Server) answerSessionPermission(req SessionKeysRequest, source string) (SessionResult, error) {
	s.sessionWatches.metaMu.Lock()
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		s.sessionWatches.metaMu.Unlock()
		return res, err
	}
	st := meta.info().Status
	switch {
	case !acceptsKeys(st.State):
		s.sessionWatches.metaMu.Unlock()
		return sessionRefusal(ReasonSessionNotLive, fmt.Sprintf("session %s is %s", req.ID, st.State)), nil
	case st.State != SessionBlocked || st.Kind != "permission":
		s.sessionWatches.metaMu.Unlock()
		return sessionRefusal(ReasonNotBlocked, fmt.Sprintf("session %s is not blocked on a permission", req.ID)), nil
	}
	c, err := nativerunner.DialHolder(dir)
	// The answer waits for the agent to move on; the status update that ends the
	// wait needs the lock.
	s.sessionWatches.metaMu.Unlock()
	if err != nil {
		return sessionRefusal(ReasonSessionNotLive, fmt.Sprintf("session %s has no live holder", req.ID)), nil
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
	return SessionResult{Session: &info}, nil
}

// sessionAnswerRefusal maps a permission answer's error to its refusal; others
// are errors.
func sessionAnswerRefusal(id string, err error) (SessionResult, error) {
	switch {
	case errors.Is(err, agentrunner.ErrRequestMismatch):
		return sessionRefusal(ReasonRequestChanged, err.Error()), nil
	case errors.Is(err, nativerunner.ErrNotApprovable), errors.Is(err, agentrunner.ErrInvalidDecision):
		return sessionRefusal(ReasonNotApprovable, err.Error()), nil
	case errors.Is(err, agentrunner.ErrNotDelivered):
		return sessionRefusal(ReasonKeysNotDelivered, err.Error()), nil
	case errors.Is(err, agentrunner.ErrNotReady):
		return sessionRefusal(ReasonNotBlocked, fmt.Sprintf("session %s is not blocked on a permission", id)), nil
	case errors.Is(err, agentrunner.ErrNotFound), errors.Is(err, agentrunner.ErrSessionDied):
		return sessionRefusal(ReasonSessionNotLive, err.Error()), nil
	}
	return SessionResult{}, fmt.Errorf("answer permission of session %s: %w", id, err)
}

// waitLeftBlocked waits up to the prompt grace for a guarded send to move the
// session out of Blocked.
func (s *Server) waitLeftBlocked(id, dir string) (SessionResult, error) {
	grace := cmp.Or(s.cfg.PromptGrace, claudeui.DefaultNudgeCaps.Grace)
	deadline := time.Now().Add(grace)
	for {
		s.sessionWatches.metaMu.Lock()
		meta, err := readSessionMeta(dir)
		s.sessionWatches.metaMu.Unlock()
		if err != nil {
			return SessionResult{}, err
		}
		if meta.info().Status.State != SessionBlocked {
			info := meta.info()
			return SessionResult{Session: &info}, nil
		}
		if time.Now().After(deadline) {
			return sessionRefusal(ReasonKeysNotDelivered, fmt.Sprintf("session %s is still blocked", id)), nil
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// validateSessionKeys checks the request's shape. It returns the bytes to send
// and how many keys or bytes the event counts, or a refusal.
func validateSessionKeys(req SessionKeysRequest) (string, int, *SessionResult) {
	bad := func(reason, msg string) (string, int, *SessionResult) {
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
		return bad(ReasonSessionBadRequest, "want exactly one of key, text or permission")
	case req.Permission != "":
		if req.Permission != string(agentrunner.DecisionAllow) && req.Permission != string(agentrunner.DecisionDeny) {
			return bad(ReasonSessionBadRequest, fmt.Sprintf("permission is %q, want allow or deny", req.Permission))
		}
		return "", 1, nil
	case req.Key != "":
		seq, ok := sessionKeyBytes[strings.ToLower(req.Key)]
		if !ok {
			return bad(ReasonSessionBadRequest, fmt.Sprintf("unknown key %q", req.Key))
		}
		return seq, 1, nil
	case strings.IndexFunc(req.Text, func(r rune) bool { return r < ' ' || r == 0x7f }) >= 0:
		return bad(ReasonSessionBadRequest, "text may not hold a control character")
	case strings.HasPrefix(req.Text, "!"):
		return bad(ReasonPromptPrefixNotAllowed, "text may not start with !")
	case len(req.Text) > maxSessionPromptBytes:
		return bad(ReasonPromptTextTooLong, fmt.Sprintf("text is over %d bytes", maxSessionPromptBytes))
	case !utf8.ValidString(req.Text):
		return bad(ReasonSessionBadRequest, "text is not valid UTF-8")
	}
	return req.Text, len(req.Text), nil
}
