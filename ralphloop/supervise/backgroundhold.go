package supervise

import (
	"fmt"
	"os"
	"time"

	"github.com/elentok/gx/backgroundtask"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

// BackgroundTaskLive re-reads, from state only, whether a background task is
// live right now: the same reading WaitForBackgroundTasks holds on, for callers
// about to act on a finish (the stall nudge, the zero-commit pause). A Codex
// session is live while its title heuristic reports Background.
func BackgroundTaskLive(d deps.Deps, p LaunchParams, sessionID string) bool {
	if p.Agent == agent.AgentCodex {
		if d.Runner == nil {
			return false
		}
		st, err := d.Runner.Status(p.SessionOrDefault())
		return err == nil && st.Background
	}
	if p.Agent != agent.AgentClaude || d.ReadBackgroundTasks == nil || sessionID == "" {
		return false
	}
	tr, err := d.ReadBackgroundTasks(p.SessionCwd, sessionID)
	if err != nil {
		return false
	}
	reading := backgroundtask.Resolve(backgroundtask.PidUnknown, tr, nil)
	if reading.AwaitingTurnEnd {
		return true
	}
	reportedAt := finishedReportTime(p)
	for _, m := range reading.Tasks {
		if m.Status != transcript.BackgroundTaskOutstandingFresh {
			continue
		}
		if !reportedAt.IsZero() && m.StartedAt.Before(reportedAt) {
			continue
		}
		if d.ReadBackgroundProcess != nil {
			if proc, err := d.ReadBackgroundProcess(sessionID, m.TaskID); err == nil && proc.Known && !proc.Alive {
				continue
			}
		}
		return true
	}
	return false
}

// BackgroundTaskHoldCap is the one cap on every live-task hold: a task, or a
// turn a task woke, that outlasts it pauses the ticket instead of waiting on.
const BackgroundTaskHoldCap = 60 * time.Minute

// BackgroundTaskHoldCapEnv replaces the cap with a Go duration: the fixture's
// knob, since an hour is out of reach of a browser flow.
const BackgroundTaskHoldCapEnv = "GX_BACKGROUND_TASK_HOLD_CAP"

// holdCap is the cap in force: the env's duration when it parses, else the const.
func holdCap() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(BackgroundTaskHoldCapEnv)); err == nil && d > 0 {
		return d
	}
	return BackgroundTaskHoldCap
}

// CodexHold counts how long a Codex session has shown a live background
// terminal. Codex reports it as working, so a finish wait never sees the
// session idle: the same cap that bounds a Claude hold bounds this.
type CodexHold struct{ heldMs int }

// Tick adds one poll of pollMs and pauses the ticket once the hold reaches the
// cap; a poll with no background terminal resets the count. Any other agent is
// ignored.
func (h *CodexHold) Tick(d deps.Deps, p LaunchParams, sessionID string, pollMs int) error {
	if p.Agent != agent.AgentCodex {
		return nil
	}
	if st, err := d.Runner.Status(p.SessionOrDefault()); err != nil || !st.Background {
		h.heldMs = 0
		return nil
	}
	h.heldMs += pollMs
	if h.heldMs >= int(holdCap()/time.Millisecond) {
		return pauseBackgroundTaskHold(p, sessionID, nil, nil)
	}
	return nil
}

// BackgroundTaskCheckEvery is how often a held task's process state is read.
const BackgroundTaskCheckEvery = 10 * time.Minute

// checkHeldTasks reads each held task's process state and logs a still-waiting
// line; it types nothing to the agent and never kills a task. A task whose
// process is gone with no notification in the transcript is released, since
// nothing will ever resolve it. A quiet output file is never a reason.
func checkHeldTasks(d deps.Deps, p LaunchParams, sessionID string, held, gone map[string]bool) (bool, error) {
	released := false
	for id := range held {
		reason := fmt.Sprintf("background task %s: state unknown", id)
		if d.ReadBackgroundProcess != nil {
			if proc, err := d.ReadBackgroundProcess(sessionID, id); err == nil && proc.Known {
				if !proc.Alive && !notified(d, p, sessionID, id) {
					delete(held, id)
					gone[id] = true
					released = true
					if err := p.LogGateEvent(events.BackgroundTaskGateReleased, sessionID, id, fmt.Sprintf("background task %s: process gone, no notification", id)); err != nil {
						return false, err
					}
					continue
				}
				reason = fmt.Sprintf("background task %s: process alive=%t, output %d bytes", id, proc.Alive, proc.OutputBytes)
			}
		}
		if err := p.LogGateEvent(events.BackgroundTaskStillWaiting, sessionID, id, "still waiting, "+reason); err != nil {
			return false, err
		}
	}
	return released, nil
}

// notified reports whether the transcript now shows the task resolved.
func notified(d deps.Deps, p LaunchParams, sessionID, taskID string) bool {
	tr, err := d.ReadBackgroundTasks(p.SessionCwd, sessionID)
	if err != nil {
		return true
	}
	for _, m := range tr.Markers {
		if m.TaskID == taskID {
			return m.Status == transcript.BackgroundTaskResolved
		}
	}
	return false
}

// finishedReportTime is when the agent reported finished, or zero when the
// ticket carries no finished report (or cannot be read).
func finishedReportTime(p LaunchParams) time.Time {
	if p.TicketPath == "" {
		return time.Time{}
	}
	t, err := schema.ParseTicket(p.TicketPath)
	if err != nil || t.IterationStatus != schema.IterationStatusFinished {
		return time.Time{}
	}
	return t.IterationFinishedAt
}

// pauseBackgroundTaskHold ends a hold that reached the cap: it closes each held
// task with a released event, so the gate watcher sees no hold on a paused
// ticket, then pauses the ticket naming the task.
func pauseBackgroundTaskHold(p LaunchParams, sessionID string, tasks []transcript.BackgroundTaskMarker, held map[string]bool) error {
	named := ""
	if len(tasks) > 0 {
		named = tasks[len(tasks)-1].TaskID
	}
	for id := range held {
		named = id
		if err := p.LogGateEvent(events.BackgroundTaskGateReleased, sessionID, id, fmt.Sprintf("background task %s: paused after %s", id, holdCap())); err != nil {
			return err
		}
	}
	subject := "background task " + named
	if named == "" {
		subject = "its background terminal"
	}
	reason := fmt.Sprintf("%s still waited on %s after %s", p.Label, subject, holdCap())
	ticketops.PauseTicket(p.EventSink(), ticketops.PauseRequest{
		ScratchDir: p.ScratchDir, EpicName: p.EpicName, Ticket: p.Ticket, Path: p.TicketPath,
		Type: events.Paused, Kind: events.BackgroundTaskGate, Reason: reason,
		Event: runlog.Event{Agent: p.Agent, Pane: p.Pane, AgentSession: sessionID, Cwd: p.SessionCwd, TaskID: named},
	})
	return &BlockedPausedError{Reason: reason}
}
