package server

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// pauseTicket is the one way the server pauses a ticket: the write, the
// EventTicketPaused event and the chat message all come from the same kind and
// reason, so the three cannot disagree. A pause that finds the ticket done (it
// lost a race with a merge) is not an error to the caller: nothing is left to pause.
// run is the paused run, zero when none launched.
func (s *Server) pauseTicket(scratchDir string, addr tickets.Address, ticketPath string, kind events.Kind, reason string, run Run) error {
	if err := ralphloop.Pause(scratchDir, addr.Epic, addr.ID, ticketPath, kind, reason, run.pauseEvent()); errors.Is(err, ralphloop.ErrPauseDone) {
		s.log.Warn("pause refused: ticket is done", "ticket", addr.String(), "kind", kind, "reason", reason)
		return nil
	} else if err != nil {
		return err
	}
	// The index must see its own write now: the watch is only a hint, and a
	// pause it misses would leave the ticket looking claimed until the backstop poll.
	if err := s.idx.refresh(s.cfg.TicketStore); err != nil {
		s.log.Warn("refresh after pause", "ticket", addr.String(), "err", err)
	}
	s.events.publish(EventTicketPaused, addr.String())
	s.announcePause(addr, ticketPath, kind, reason)
	return nil
}

// announcePause is the one place that routes a pause already written: held for
// recovery, held for herdr's digest, or sent to chat now. Every caller that has
// just written a pause goes through it; a recovery hold's release is the only
// path that skips the recovery choice.
func (s *Server) announcePause(addr tickets.Address, ticketPath string, kind events.Kind, reason string) {
	switch {
	case s.holdPauseForRecovery(addr, ticketPath, kind, reason):
	case kind.CauseHerdr() && s.herdr.isUnavailable():
		s.holdPause(addr, ralphloop.PauseStatus(kind), kind, reason)
	default:
		s.sendPause(addr, ticketPath, kind, reason)
	}
}

// notifyPause releases a pause recovery was holding: herdr's outage is still
// checked, the recovery hold is not.
func (s *Server) notifyPause(addr tickets.Address, ticketPath string, kind events.Kind, reason string) {
	if kind.CauseHerdr() && s.herdr.isUnavailable() {
		s.holdPause(addr, ralphloop.PauseStatus(kind), kind, reason)
		return
	}
	s.sendPause(addr, ticketPath, kind, reason)
}

// sendPause sends the one chat message for a pause.
func (s *Server) sendPause(addr tickets.Address, ticketPath string, kind events.Kind, reason string) {
	status := ralphloop.PauseStatus(kind)
	s.chat.Pause(addr.Project, s.chatOverride(addr.Project), addr.Epic, ticketPath, addr.ID, status, kind, pauseChatDetail(ticketPath, kind, reason), s.epicCounts(addr.Project, addr.Epic))
}

// pauseChatDetail is the chat line for a pause. A self-reported pause shows the
// agent's own question summary, which says more than the generic reason. It
// falls back to the generic line when the ticket has no open question.
func pauseChatDetail(ticketPath string, kind events.Kind, reason string) string {
	if kind == events.SelfReported && ticketPath != "" {
		if evs, err := tickets.ReadLog(ticketPath); err == nil {
			if q := tickets.OpenQuestion(evs); q != nil && q.Summary != "" {
				return q.Summary
			}
		}
	}
	return fmt.Sprintf("%s: %s", kind, reason)
}

// finishSink carries the finish path's chat-worthy events the outcome cannot
// show: a merge conflict's resolution starting, and its child pausing.
func (s *Server) finishSink(addr tickets.Address, ticketPath string) ralphloop.EventSink {
	return s.chat.FinishSink(addr.Project, s.chatOverride(addr.Project), addr.Epic, ticketPath, addr.ID, func() ralphloop.EpicCounts {
		return s.epicCounts(addr.Project, addr.Epic)
	})
}

// epicCounts tallies the epic from disk, fresh: a pause has just rewritten a
// ticket, so the scanned view may not have it yet. Zero when the epic cannot be read.
func (s *Server) epicCounts(project, epic string) ralphloop.EpicCounts {
	e, ok := s.freshEpic(project, epic)
	if !ok {
		return ralphloop.EpicCounts{}
	}
	return ralphloop.CountsOf(e)
}

// freshEpic loads one epic straight from its project's directory.
func (s *Server) freshEpic(project, epic string) (tickets.Epic, bool) {
	dir, err := s.projectDir(project)
	if err != nil {
		return tickets.Epic{}, false
	}
	epics, err := tickets.LoadResolved(project, dir)
	if err != nil {
		return tickets.Epic{}, false
	}
	for _, e := range epics {
		if filepath.Base(e.Path) == epic {
			return e, true
		}
	}
	return tickets.Epic{}, false
}
