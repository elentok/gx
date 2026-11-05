package ticketwrite

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
)

// Nudge types req.Text into the live pane of a ticket's iteration. It is
// the only way to nudge a pane: every nudge is rate-limited, written to the
// epic's run log and streamed.
func (w *Writer) Nudge(req wire.QueueRequest) (wire.QueueResult, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return queue.Refusal(wire.ReasonTextRequired, "text to type into the pane is required"), nil
	}
	h := w.d.Host
	return w.ResolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		address := ref.Addr.String()
		if r, err := w.d.Harness.IterationRunner(ref.Addr); err == nil {
			if reason, held := h.HerdrHolds(r); held {
				return queue.Refusal(wire.ReasonHerdrUnavailable, reason), nil
			}
		}
		if res, refused := h.BudgetRefusal(); refused {
			return res, nil
		}
		it, live := h.LiveIteration(address)
		if !live {
			return queue.Refusal(wire.ReasonIterationNotLive, "no live iteration for "+address), nil
		}
		now := time.Now()
		if !w.nudges.Allow(address, now) {
			return queue.Refusal(wire.ReasonRateLimited, fmt.Sprintf("more than %d nudges to %s within %s", NudgeLimit, address, NudgeWindow)), nil
		}
		if err := it.Runner.Prompt(it.Session, text); err != nil {
			if errors.Is(err, agentrunner.ErrNotReady) {
				return queue.Refusal(wire.ReasonAgentNotReady, err.Error()), nil
			}
			return wire.QueueResult{}, fmt.Errorf("nudge %s: %w", address, err)
		}
		err := runlog.AppendEvent(ref.ProjectDir, ref.Addr.Epic, runlog.Event{
			Time: now, Type: string(events.Nudged), Ticket: ref.Addr.ID, Address: address,
			Pane: it.Info.Pane, Text: text,
		})
		if err != nil {
			return wire.QueueResult{}, fmt.Errorf("logging nudge of %s: %w", address, err)
		}
		w.d.Events.Publish(wire.EventTicketNudged, address)
		if !w.d.Registry.Has(address) {
			w.WatchNudgedPane(ref, it)
		}
		return wire.QueueResult{}, nil
	})
}

// nudgeWatchStart bounds how long a watched pane gets to start working on the
// nudge; nudgeWatchPoll is one wait for it to settle again.
const (
	nudgeWatchStart = 30 * time.Second
	nudgeWatchPoll  = 5 * time.Minute
)

// WatchNudgedPane tracks a pane a nudge set working after its run ended (a
// paused ticket's pane, as recovery's R7 nudges): until it settles it counts
// as a run, so the budget meters it and the hard limit stops it. It is never
// finished or merged; the paused ticket stays with whoever handles the pause.
func (w *Writer) WatchNudgedPane(ref queue.TicketRef, it LiveIter) {
	t := runs.TrackedRun{
		Run:  runs.Run{Address: ref.Addr.String(), Runner: it.Runner.Name(), Session: it.Session},
		Root: ref.Root().String(), TicketPath: ref.Ticket.Path, Cwd: it.Info.Worktree, StartedAt: time.Now(), Watched: true,
	}
	r := it.Runner
	w.d.Registry.Put(t)
	w.d.Guards.GoSettle(func() {
		defer w.d.Registry.DeleteWatched(t)
		// The nudge may not have shown as working yet; a pane already idle
		// again by the deadline simply settles below.
		_, err := r.Wait(t.Session, []agentrunner.State{agentrunner.StateWorking}, nudgeWatchStart)
		for err == nil || errors.Is(err, agentrunner.ErrTimeout) {
			if _, err = r.Wait(t.Session, []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone}, nudgeWatchPoll); err == nil {
				break
			}
		}
		if stopping, first := w.d.Registry.ClaimBudgetStop(t.Address); stopping && first {
			if perr := w.d.Budget.PauseKilled(t); perr != nil {
				w.d.Log.Error("budget kill", "ticket", t.Address, "err", perr)
			}
		}
	})
}
