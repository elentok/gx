package sessions

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/nativerunner/holder"
)

// InterruptSession ends a Working session's current turn and keeps the session,
// with the harness's own interrupt key (Esc for Codex, Ctrl+C for Claude). The holder protocol has no interrupt verb: its
// keys reach claude as typed.
func (mg *Manager) InterruptSession(req wire.SessionRequest) (wire.SessionResult, error) {
	mg.Watches.MetaMu.Lock()
	defer mg.Watches.MetaMu.Unlock()
	dir, meta, res, err := mg.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if st := meta.Info().Status; st.State != wire.SessionWorking {
		return Refusal(wire.ReasonSessionNotWorking, fmt.Sprintf("session %s is %s, not working", req.ID, st.State)), nil
	}
	c, err := holder.DialHolder(dir)
	if err == nil {
		err = c.SendKeys(mg.projects.TypingFor(meta.Harness).InterruptKey())
	}
	if err != nil {
		return wire.SessionResult{}, fmt.Errorf("interrupt session %s: %w", req.ID, err)
	}
	info := meta.Info()
	return wire.SessionResult{Session: &info}, nil
}

// SessionStateToken is the web's proof of which session state it acted on: the
// id, the status and when the status last changed. A stop or Restore carrying
// an older one is refused as stale.
func SessionStateToken(s wire.SessionInfo) string {
	return fmt.Sprintf("%s:%s:%d", s.ID, s.Status.State, s.StatusChangedAt.UnixNano())
}

// SessionWriteDesignWeb is sessionWrite for the web mux: only a design session,
// and only with the state token of the session the page saw.
func (mg *Manager) SessionWriteDesignWeb(do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return sessionWriteAs(mg.CheckSteppedWrite, do)
}

// CheckSteppedWrite returns the refusal for a web write on a session that is not
// a stepped session or whose state moved since the page saw it.
func (mg *Manager) CheckSteppedWrite(req wire.SessionRequest) (wire.SessionResult, error) {
	_, meta, res, err := mg.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if !meta.IsStepped() {
		return Refusal(wire.ReasonSessionNotDesign, fmt.Sprintf("session %s is not a stepped session", req.ID)), nil
	}
	if req.State != SessionStateToken(meta.Info()) {
		return Refusal(wire.ReasonStale, "the session changed; review its current state"), nil
	}
	return wire.SessionResult{}, nil
}

// SessionRowToken is the web's proof of the row a stop or remove was selected
// on: the id, when the current run began, and whether it is live or Exited. A
// Working/Idle flip leaves it alone. The time is the run_started_at string the
// page already holds.
func SessionRowToken(s wire.SessionInfo) string {
	state := "live"
	if s.Status.State == wire.SessionExited {
		state = "exited"
	}
	return fmt.Sprintf("%s:%s:%s", s.ID, s.RunStartedAt.Format(time.RFC3339Nano), state)
}

// checkAgentWrite returns the refusal for a web stop or remove of a Queue
// session, which ralph-loop drives, or of a row that changed since it was
// selected. It compares only SessionRowToken.
func (mg *Manager) checkAgentWrite(t wire.SessionTarget) (wire.SessionResult, error) {
	// A Queue agent has no state dir here, so its id prefix is the only tell.
	if strings.HasPrefix(t.ID, queue.AgentIDPrefix) {
		return Refusal(wire.ReasonSessionQueue, fmt.Sprintf("session %s is a Queue agent; ralph-loop drives it", t.ID)), nil
	}
	_, meta, res, err := mg.SessionDir(t.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if info := meta.Info(); t.State != SessionRowToken(info) {
		return Refusal(wire.ReasonStale, wire.StaleRowMessage), nil
	}
	return wire.SessionResult{}, nil
}

// SessionWriteInterruptWeb is sessionWrite for a web interrupt: any session
// except a Queue agent, which ralph-loop drives, and only with the state token
// of the session the page saw.
func (mg *Manager) SessionWriteInterruptWeb(do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return sessionWriteAs(mg.checkInterruptWrite, do)
}

// checkInterruptWrite returns the refusal for a web interrupt of a Queue
// session or of a session whose state moved since the page saw it.
func (mg *Manager) checkInterruptWrite(req wire.SessionRequest) (wire.SessionResult, error) {
	_, meta, res, err := mg.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Info().Queue {
		return Refusal(wire.ReasonSessionQueue, fmt.Sprintf("session %s is a Queue agent; ralph-loop drives it", req.ID)), nil
	}
	if req.State != SessionStateToken(meta.Info()) {
		return Refusal(wire.ReasonStale, "the session changed; review its current state"), nil
	}
	return wire.SessionResult{}, nil
}
