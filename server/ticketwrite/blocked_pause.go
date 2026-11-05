package ticketwrite

import (
	"errors"
	"fmt"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// WaitsOnBlockedPause reports whether tk is paused blocked-pane with a tracked,
// waiting run behind it, on any runner.
func (w *Writer) WaitsOnBlockedPause(tk schema.Ticket, address string) bool {
	if w.d.Registry == nil || tk.PauseReason != schema.PauseBlockedPane {
		return false
	}
	_, ok := w.d.Registry.Tracked(address)
	return ok
}

// LeaveBlocked returns a ticket paused blocked-pane to claimed and lets the
// same session keep going, as an answer to a question does. Any other ticket is
// left alone. The answer verb and the unblock watcher both call it; the first
// one in does the work.
func (w *Writer) LeaveBlocked(address string) error {
	if w.d.Registry == nil {
		return nil
	}
	w.unblockMu.Lock()
	defer w.unblockMu.Unlock()
	run, ok := w.d.Registry.Tracked(address)
	if !ok || run.WaitingSince.IsZero() {
		return nil
	}
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	_, t, ok := w.FreshTicket(addr)
	if !ok {
		return nil
	}
	tk, err := schema.ParseTicket(t.Path)
	if err != nil {
		return err
	}
	if !w.WaitsOnBlockedPause(tk, address) || !tk.PausedAsking() {
		return nil
	}
	if err := ralphloop.Claim(t.Path); err != nil {
		return fmt.Errorf("claim %s: %w", address, err)
	}
	if err := w.d.Host.ResettleWaiting(run); err != nil {
		return fmt.Errorf("settle %s: %w", address, err)
	}
	w.d.Index.Rescan()
	return nil
}

// RetryBlocked ends the blocked pause of a PTY or herdr run and interrupts its
// agent, and reports whether it did. A headless run is left alone. The pause ends
// first, through the same path as the answer verb, so the unblock watch seeing
// the interrupt finds nothing left to do. Interrupt waits for idle, which a raw
// Esc would not: the retry prompt then lands on an agent that is ready for it.
func (w *Writer) RetryBlocked(address string) (bool, error) {
	run, ok := w.d.Registry.Tracked(address)
	if !ok {
		return false, nil
	}
	runner, err := w.d.Harness.RunnerOf(run.Run)
	if err != nil {
		return false, err
	}
	if runner.Name() == string(agentrunner.ChoiceHeadless) {
		return false, nil
	}
	if err := w.LeaveBlocked(address); err != nil {
		return false, err
	}
	if err := runner.Interrupt(run.Session); err != nil {
		return false, fmt.Errorf("interrupt %s: %w", address, err)
	}
	return true, nil
}

// WatchUnblock returns the ticket to claimed when its blocked agent leaves the
// blocked state by any route: an answer, an interrupt, a cancelled request, or
// the agent running on or finishing by itself. It ends when the run stops
// waiting or the session is gone; a gone agent is the resume path's to handle.
func (w *Writer) WatchUnblock(r agentrunner.Runner, sess agentrunner.Session, address string) {
	until := []agentrunner.State{agentrunner.StateWorking, agentrunner.StateIdle, agentrunner.StateDone}
	for {
		_, err := r.Wait(sess, until, w.d.BlockWatchPoll)
		if errors.Is(err, agentrunner.ErrTimeout) {
			if run, ok := w.d.Registry.Tracked(address); !ok || run.WaitingSince.IsZero() {
				return
			}
			continue
		}
		if err != nil {
			return
		}
		if err := w.LeaveBlocked(address); err != nil {
			w.d.Log.Warn("leave blocked", "ticket", address, "err", err)
		}
		return
	}
}
