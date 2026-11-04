package server

import (
	"fmt"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/nativerunner"
)

// interruptSession ends a Working session's current turn and keeps the session,
// with the Ctrl+C the TUI uses. The holder protocol has no interrupt verb: its
// keys reach claude as typed.
func (s *Server) interruptSession(req wire.SessionRequest) (wire.SessionResult, error) {
	s.sessionWatches.metaMu.Lock()
	defer s.sessionWatches.metaMu.Unlock()
	dir, meta, res, err := s.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if st := meta.info().Status; st.State != wire.SessionWorking {
		return sessionRefusal(wire.ReasonSessionNotWorking, fmt.Sprintf("session %s is %s, not working", req.ID, st.State)), nil
	}
	c, err := nativerunner.DialHolder(dir)
	if err == nil {
		err = c.SendKeys("\x03")
	}
	if err != nil {
		return wire.SessionResult{}, fmt.Errorf("interrupt session %s: %w", req.ID, err)
	}
	info := meta.info()
	return wire.SessionResult{Session: &info}, nil
}
