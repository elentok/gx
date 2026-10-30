package server

import (
	"errors"
	"fmt"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// waitsOnBlockedPause reports whether tk is paused blocked-pane with a tracked,
// waiting run behind it, on any runner.
func (s *Server) waitsOnBlockedPause(tk schema.Ticket, address string) bool {
	if s.registry == nil || tk.PauseReason != schema.PauseBlockedPane {
		return false
	}
	_, ok := s.registry.tracked(address)
	return ok
}

// leaveBlocked returns a ticket paused blocked-pane to claimed and lets the
// same session keep going, as an answer to a question does. Any other ticket is
// left alone. The answer verb and the unblock watcher both call it; the first
// one in does the work.
func (s *Server) leaveBlocked(address string) error {
	if s.registry == nil {
		return nil
	}
	s.unblockMu.Lock()
	defer s.unblockMu.Unlock()
	run, ok := s.registry.tracked(address)
	if !ok || run.WaitingSince.IsZero() {
		return nil
	}
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	_, t, ok := s.freshTicket(addr)
	if !ok {
		return nil
	}
	tk, err := schema.ParseTicket(t.Path)
	if err != nil {
		return err
	}
	if !s.waitsOnBlockedPause(tk, address) || !tk.PausedAsking() {
		return nil
	}
	if err := ralphloop.Claim(t.Path); err != nil {
		return fmt.Errorf("claim %s: %w", address, err)
	}
	if err := s.resettleWaiting(run); err != nil {
		return fmt.Errorf("settle %s: %w", address, err)
	}
	s.rescan()
	return nil
}

// watchUnblock returns the ticket to claimed when its blocked agent leaves the
// blocked state by any route: an answer, an interrupt, a cancelled request, or
// the agent running on or finishing by itself. It ends when the run stops
// waiting or the session is gone; a gone agent is the resume path's to handle.
func (s *Server) watchUnblock(r agentrunner.Runner, sess agentrunner.Session, address string) {
	until := []agentrunner.State{agentrunner.StateWorking, agentrunner.StateIdle, agentrunner.StateDone}
	for {
		_, err := r.Wait(sess, until, blockWatchPoll)
		if errors.Is(err, agentrunner.ErrTimeout) {
			if run, ok := s.registry.tracked(address); !ok || run.WaitingSince.IsZero() {
				return
			}
			continue
		}
		if err != nil {
			return
		}
		if err := s.leaveBlocked(address); err != nil {
			s.log.Warn("leave blocked", "ticket", address, "err", err)
		}
		return
	}
}
