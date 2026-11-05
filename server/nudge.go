package server

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/ticketwrite"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
)

// ticketNudge types req.Text into the live pane of a ticket's iteration. It is
// the only way to nudge a pane: every nudge is rate-limited, written to the
// epic's run log and streamed.
func (s *Server) ticketNudge(req wire.QueueRequest) (wire.QueueResult, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return queue.Refusal(wire.ReasonTextRequired, "text to type into the pane is required"), nil
	}
	return s.resolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		address := ref.Addr.String()
		if r, err := s.harness.IterationRunner(ref.Addr); err == nil {
			if reason, held := s.herdrHolds(r); held {
				return queue.Refusal(wire.ReasonHerdrUnavailable, reason), nil
			}
		}
		if res, refused := s.budgetRefusal(); refused {
			return res, nil
		}
		it, live := s.liveIteration(address)
		if !live {
			return queue.Refusal(wire.ReasonIterationNotLive, "no live iteration for "+address), nil
		}
		now := time.Now()
		if !s.nudges.Allow(address, now) {
			return queue.Refusal(wire.ReasonRateLimited, fmt.Sprintf("more than %d nudges to %s within %s", ticketwrite.NudgeLimit, address, ticketwrite.NudgeWindow)), nil
		}
		if err := it.runner.Prompt(it.session, text); err != nil {
			if errors.Is(err, agentrunner.ErrNotReady) {
				return queue.Refusal(wire.ReasonAgentNotReady, err.Error()), nil
			}
			return wire.QueueResult{}, fmt.Errorf("nudge %s: %w", address, err)
		}
		err := ralphloop.AppendEvent(ref.ProjectDir, ref.Addr.Epic, ralphloop.Event{
			Time: now, Type: string(events.Nudged), Ticket: ref.Addr.ID, Address: address,
			Pane: it.info.Pane, Text: text,
		})
		if err != nil {
			return wire.QueueResult{}, fmt.Errorf("logging nudge of %s: %w", address, err)
		}
		s.events.Publish(wire.EventTicketNudged, address)
		if !s.registry.Has(address) {
			s.watchNudgedPane(ref, it)
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

// watchNudgedPane tracks a pane a nudge set working after its run ended (a
// paused ticket's pane, as recovery's R7 nudges): until it settles it counts
// as a run, so the budget meters it and the hard limit stops it. It is never
// finished or merged; the paused ticket stays with whoever handles the pause.
func (s *Server) watchNudgedPane(ref queue.TicketRef, it liveIter) {
	t := runs.TrackedRun{
		Run:  runs.Run{Address: ref.Addr.String(), Runner: it.runner.Name(), Session: it.session},
		Root: ref.Root().String(), TicketPath: ref.Ticket.Path, Cwd: it.info.Worktree, StartedAt: time.Now(), Watched: true,
	}
	r := it.runner
	s.registry.Put(t)
	s.guards.GoSettle(func() {
		defer s.registry.DeleteWatched(t)
		// The nudge may not have shown as working yet; a pane already idle
		// again by the deadline simply settles below.
		_, err := r.Wait(t.Session, []agentrunner.State{agentrunner.StateWorking}, nudgeWatchStart)
		for err == nil || errors.Is(err, agentrunner.ErrTimeout) {
			if _, err = r.Wait(t.Session, []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone}, nudgeWatchPoll); err == nil {
				break
			}
		}
		if stopping, first := s.registry.ClaimBudgetStop(t.Address); stopping && first {
			if perr := s.budget.PauseKilled(t); perr != nil {
				s.log.Error("budget kill", "ticket", t.Address, "err", perr)
			}
		}
	})
}
