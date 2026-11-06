package ticketwrite

import (
	"errors"
	"path/filepath"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/schedule"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Pause is the one way the server pauses a ticket: the write, the
// EventTicketPaused event and the chat message all come from the same kind and
// reason, so the three cannot disagree. A pause that finds the ticket done (it
// lost a race with a merge) is not an error to the caller: nothing is left to pause.
// run is the paused run, zero when none launched.
func (w *Writer) Pause(scratchDir string, addr tickets.Address, ticketPath string, kind events.Kind, reason string, run runs.Run) error {
	if err := ticketops.Pause(scratchDir, addr.Epic, addr.ID, ticketPath, kind, reason, run.PauseEvent()); errors.Is(err, ticketops.ErrPauseDone) {
		w.d.Log.Warn("pause refused: ticket is done", "ticket", addr.String(), "kind", kind, "reason", reason)
		return nil
	} else if err != nil {
		return err
	}
	// The index must see its own write now: the watch is only a hint, and a
	// pause it misses would leave the ticket looking claimed until the backstop poll.
	if err := w.d.Index.Refresh(); err != nil {
		w.d.Log.Warn("refresh after pause", "ticket", addr.String(), "err", err)
	}
	w.d.Events.Publish(wire.EventTicketPaused, addr.String())
	w.AnnouncePause(addr, ticketPath, kind, reason)
	return nil
}

// PauseAs pauses the ticket at address, resolving its project and file first.
func (w *Writer) PauseAs(address string, kind events.Kind, reason string) error {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	ref, ok, err := w.FindTicket(address)
	if err != nil || !ok {
		return errors.Join(err, errors.New("no ticket "+address))
	}
	return w.Pause(ref.ProjectDir, addr, ref.Ticket.Path, kind, reason, runs.Run{})
}

// AnnouncePause is the one place that routes a pause already written: held for
// recovery, held for herdr's digest, or sent to chat now. Every caller that has
// just written a pause goes through it; a recovery hold's release is the only
// path that skips the recovery choice.
func (w *Writer) AnnouncePause(addr tickets.Address, ticketPath string, kind events.Kind, reason string) {
	switch {
	case w.d.Host.HoldPauseForRecovery(addr, ticketPath, kind, reason):
	case kind.CauseHerdr() && w.d.Host.HerdrUnavailable():
		w.holdPause(addr, ticketPath, ticketops.PauseStatus(kind), kind, reason)
	default:
		w.sendPause(addr, ticketPath, kind, reason)
	}
}

// NotifyPause releases a pause recovery was holding: herdr's outage is still
// checked, the recovery hold is not.
func (w *Writer) NotifyPause(addr tickets.Address, ticketPath string, kind events.Kind, reason string) {
	if kind.CauseHerdr() && w.d.Host.HerdrUnavailable() {
		w.holdPause(addr, ticketPath, ticketops.PauseStatus(kind), kind, reason)
		return
	}
	w.sendPause(addr, ticketPath, kind, reason)
}

// sendPause sends the one chat message for a pause.
func (w *Writer) sendPause(addr tickets.Address, ticketPath string, kind events.Kind, reason string) {
	status := ticketops.PauseStatus(kind)
	w.d.Chat.Pause(addr.Project, w.d.Projects.ChatOverride(addr.Project), addr.Epic, ticketPath, addr.ID, status, kind, PauseChatDetail(ticketPath, kind, reason), w.epicCounts(addr.Project, addr.Epic))
}

// FinishSink carries the finish path's chat-worthy events the outcome cannot
// show: a merge conflict's resolution starting, and its child pausing.
func (w *Writer) FinishSink(addr tickets.Address, ticketPath string) runlog.EventSink {
	return w.d.Chat.FinishSink(addr.Project, w.d.Projects.ChatOverride(addr.Project), addr.Epic, ticketPath, addr.ID, func() schedule.EpicCounts {
		return w.epicCounts(addr.Project, addr.Epic)
	})
}

// epicCounts tallies the epic from disk, fresh: a pause has just rewritten a
// ticket, so the scanned view may not have it yet. Zero when the epic cannot be read.
func (w *Writer) epicCounts(project, epic string) schedule.EpicCounts {
	e, ok := w.FreshEpic(project, epic)
	if !ok {
		return schedule.EpicCounts{}
	}
	return schedule.CountsOf(e)
}

// FreshEpic loads one epic straight from its project's directory.
func (w *Writer) FreshEpic(project, epic string) (tickets.Epic, bool) {
	dir, err := w.d.Projects.Dir(project)
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

// FreshTicket loads addr's ticket, and its epic, straight from disk.
func (w *Writer) FreshTicket(addr tickets.Address) (tickets.Epic, tickets.Ticket, bool) {
	e, ok := w.FreshEpic(addr.Project, addr.Epic)
	if !ok {
		return tickets.Epic{}, tickets.Ticket{}, false
	}
	for _, t := range e.Tickets {
		if t.Identifier == addr.ID {
			return e, t, true
		}
	}
	return tickets.Epic{}, tickets.Ticket{}, false
}
