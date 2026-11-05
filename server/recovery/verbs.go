package recovery

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// recoveryVerbs lets a remedy call the server's own verbs. Each call goes
// through the same ticket write as a person's, so refusals, the merge lock and
// events are identical; only the actor differs.
type recoveryVerbs struct{ rec *Recovery }

// Do calls one server verb the way a remedy does. It is exported for tests.
func (rec *Recovery) Do(c recovery.Call) (recovery.Result, error) {
	return recoveryVerbs{rec}.Do(c)
}

// recoveryVerbHandlers are the server's verbs by name; each takes the request
// a call makes. A verb that reads the actor gets recoveryCtx.
var recoveryCtx = queue.Ctx{Actor: recovery.ActorRecovery}

var recoveryVerbHandlers = map[string]func(rec *Recovery, req wire.QueueRequest, c recovery.Call) (wire.QueueResult, error){
	recovery.VerbPause: func(rec *Recovery, req wire.QueueRequest, c recovery.Call) (wire.QueueResult, error) {
		req.PauseReason = c.Text
		return rec.d.Writer.PauseRequest(req)
	},
	recovery.VerbRelaunch: func(rec *Recovery, req wire.QueueRequest, _ recovery.Call) (wire.QueueResult, error) {
		return rec.d.Writer.Relaunch(req, recoveryCtx)
	},
	recovery.VerbCommitlessDone: func(rec *Recovery, req wire.QueueRequest, _ recovery.Call) (wire.QueueResult, error) {
		return rec.ticketCommitlessDone(req)
	},
	recovery.VerbNudge: func(rec *Recovery, req wire.QueueRequest, c recovery.Call) (wire.QueueResult, error) {
		req.Text = c.Text
		return rec.d.Writer.Nudge(req)
	},
	recovery.VerbClosePane:   (*Recovery).recoveryClosePane,
	recovery.VerbWait:        (*Recovery).recoveryWait,
	recovery.VerbReleaseGate: (*Recovery).recoveryReleaseGate,
	recovery.VerbFinish:      (*Recovery).recoveryFinish,
	recovery.VerbSetParent:   (*Recovery).recoverySetParent,
}

func (v recoveryVerbs) Do(c recovery.Call) (recovery.Result, error) {
	h, ok := recoveryVerbHandlers[c.Verb]
	if !ok {
		return recovery.Result{}, fmt.Errorf("unknown recovery verb %q", c.Verb)
	}
	res, err := h(v.rec, wire.QueueRequest{Address: c.Address}, c)
	return recoveryResult(res), err
}

func (rec *Recovery) recoveryClosePane(req wire.QueueRequest, _ recovery.Call) (wire.QueueResult, error) {
	return rec.d.Writer.ResolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		it, live := rec.d.Host.LiveIteration(ref.Addr.String())
		if !live {
			return wire.QueueResult{}, nil
		}
		if err := it.Runner.Stop(it.Session); err != nil {
			return wire.QueueResult{}, fmt.Errorf("close pane of %s: %w", ref.Addr, err)
		}
		if err := agentrunner.Cleanup(it.Runner, it.Session.Label); err != nil {
			return wire.QueueResult{}, fmt.Errorf("clean up %s: %w", ref.Addr, err)
		}
		return wire.QueueResult{}, nil
	})
}

// DefaultCompactRewaitTimeout is R7's one extended wait, well past the loop's
// own compaction wait so a slow compaction can still finish inside it.
const DefaultCompactRewaitTimeout = 15 * time.Minute

func (rec *Recovery) recoveryWait(req wire.QueueRequest, _ recovery.Call) (wire.QueueResult, error) {
	return rec.d.Writer.ResolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		it, live := rec.d.Host.LiveIteration(ref.Addr.String())
		if !live {
			return queue.Refusal(wire.ReasonIterationNotLive, "no live iteration for "+ref.Addr.String()), nil
		}
		timeout := rec.d.CompactRewaitTimeout
		if timeout == 0 {
			timeout = DefaultCompactRewaitTimeout
		}
		if _, err := it.Runner.Wait(it.Session, []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone}, timeout); err != nil {
			return wire.QueueResult{}, fmt.Errorf("wait for %s: %w", ref.Addr, err)
		}
		return wire.QueueResult{}, nil
	})
}

// AgentState is the run's agent state, read fresh through the run's runner.
// idle is true for an agent that is not working: idle, or done.
func (rec *Recovery) AgentState(addr tickets.Address) (state agentrunner.State, idle bool, err error) {
	runner, sess, ok, err := rec.d.Harness.LiveSession(addr)
	if err != nil || !ok {
		return "", false, cmp.Or(err, fmt.Errorf("no agent for %s", addr))
	}
	st, err := runner.Status(sess)
	if err != nil {
		return "", false, err
	}
	return st.State, st.Waiting(), nil
}

// recoveryReleaseGate releases a held gate on an idle agent. Zero commits and
// a clean worktree with commits ahead both let the finish run as it would:
// paused zero-commit, or merged. A dirty worktree with commits ahead must not
// merge unfinished work, so its finish pauses background-task-gate instead.
func (rec *Recovery) recoveryReleaseGate(req wire.QueueRequest, _ recovery.Call) (wire.QueueResult, error) {
	return rec.d.Writer.ResolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		run, ok := rec.d.Registry.Tracked(ref.Addr.String())
		if !ok {
			return queue.Refusal(wire.ReasonIterationNotLive, "no live iteration for "+ref.Addr.String()), nil
		}
		state, idle, err := rec.AgentState(ref.Addr)
		if err != nil {
			return wire.QueueResult{}, fmt.Errorf("read agent of %s: %w", ref.Addr, err)
		}
		if !idle {
			return queue.Refusal(wire.ReasonAgentBusy, ref.Addr.String()+" agent is "+string(state)), nil
		}
		_, _, worktree := ralphloop.IterationIdentity(ref.Addr.Epic, ref.Addr.ID, rec.d.Projects.WorktreeDir(ref.Addr.Project))
		ahead, err := git.CommitsAhead(worktree, run.Base, "HEAD")
		if err != nil {
			return wire.QueueResult{}, fmt.Errorf("commits ahead in %s: %w", worktree, err)
		}
		pause := ""
		if ahead > 0 {
			dirty, err := git.UncommittedChanges(worktree)
			if err != nil {
				return wire.QueueResult{}, fmt.Errorf("status of %s: %w", worktree, err)
			}
			if len(dirty) > 0 {
				pause = gatePauseReason(dirty)
			}
		}
		rec.d.Registry.ReleaseGate(ref.Addr.String(), pause)
		return wire.QueueResult{}, nil
	})
}

func gatePauseReason(dirty []git.Change) string {
	paths := make([]string, len(dirty))
	for i, c := range dirty {
		paths[i] = c.Path
	}
	return "background-task gate released on an idle agent, but the worktree has uncommitted changes: " + strings.Join(paths, ", ")
}

// PauseReleasedGate writes the pause release-gate left for a run in place of its
// finish, leaving worktree and pane for a person. False when there is none.
func (rec *Recovery) PauseReleasedGate(scratchDir string, addr tickets.Address, ticketPath string, run runs.Run) bool {
	reason := rec.d.Registry.GatePause(addr.String())
	if reason == "" {
		return false
	}
	if err := rec.d.Writer.Pause(scratchDir, addr, ticketPath, events.BackgroundTaskGate, reason, run); err != nil {
		rec.d.Log.Error("pause released gate", "ticket", addr.String(), "err", err)
	}
	return true
}

// finishWaitTimeout bounds Finish: a released gate's finish only re-checks
// idle and merges, so a run still live well past that is stuck elsewhere.
const finishWaitTimeout = 10 * time.Minute

// finishPoll paces Finish's registry reads; a var so tests can shorten it.
var finishPoll = time.Second

// recoveryFinish holds no ticket lock while it waits: the merge it waits for
// needs it.
func (rec *Recovery) recoveryFinish(req wire.QueueRequest, _ recovery.Call) (wire.QueueResult, error) {
	if !rec.d.Registry.WaitGone(req.Address, finishWaitTimeout, finishPoll) {
		return queue.Refusal(wire.ReasonFinishTimedOut, req.Address+" is still running after its gate was released"), nil
	}
	return wire.QueueResult{}, nil
}

// recoverySetParent re-checks and writes under the epic lock `gx tickets set
// --parent` takes, so a re-parent between the scan and the write is seen, not
// clobbered.
func (rec *Recovery) recoverySetParent(req wire.QueueRequest, c recovery.Call) (wire.QueueResult, error) {
	address, parent := req.Address, c.Parent
	return rec.d.Writer.ResolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		epic, unlock, err := tickets.LoadLockedEpic(ref.Epic.Path)
		if err != nil {
			return wire.QueueResult{}, fmt.Errorf("lock epic of %s: %w", ref.Addr, err)
		}
		defer unlock()
		i := slices.IndexFunc(epic.Tickets, func(t tickets.Ticket) bool { return t.Identifier == ref.Addr.ID })
		if i < 0 {
			return queue.Refusal(wire.ReasonUnknownTicket, "no ticket "+address), nil
		}
		target := &epic.Tickets[i]
		if _, defect := recovery.ParentDefect(target.Identifier, target.Parent); !defect {
			return queue.Refusal(wire.ReasonParentChanged, address+" has a parent its ID allows"), nil
		}
		target.Parent = &parent
		if err := epic.ValidateParentGraph(); err != nil {
			return queue.Refusal(wire.ReasonInvalidParent, err.Error()), nil
		}
		err = schema.UpdateTicket(target.Path, func(t *schema.Ticket) {
			id := schema.TicketID(parent)
			t.Parent = &id
		})
		if err != nil {
			return wire.QueueResult{}, fmt.Errorf("set parent of %s: %w", ref.Addr, err)
		}
		return wire.QueueResult{}, nil
	})
}

func (v recoveryVerbs) LaunchPrompt(address string) (string, error) {
	ref, ok, err := v.rec.d.Writer.FindTicket(address)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no ticket %s", address)
	}
	return LaunchPrompt(v.rec.d.Harness.ResolveTicket(ref.Addr, ref.Ticket).Harness, LaunchSkill(ref.Ticket), InvestigatePrompt(ref.Ticket), address), nil
}

// ticketCommitlessDone marks a ticket done with no commits of its own, for work
// that merged inside another ticket's commits. Commitless takes it out of
// reconcile's hands for good, which is why only an approved proposal calls it.
func (rec *Recovery) ticketCommitlessDone(req wire.QueueRequest) (wire.QueueResult, error) {
	return rec.d.Writer.TicketWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		err := schema.UpdateTicket(ref.Ticket.Path, func(t *schema.Ticket) {
			t.Status, t.IterationStatus, t.Commitless = schema.StatusDone, schema.IterationStatusFinished, true
		})
		if err != nil {
			return wire.QueueResult{}, fmt.Errorf("commitless-done %s: %w", ref.Addr, err)
		}
		rec.d.Writer.StampFinished(ref.Addr, ref.Ticket.Path, false)
		rec.d.Events.Publish(wire.EventTicketDone, ref.Addr.String())
		return wire.QueueResult{}, nil
	})
}

func recoveryResult(r wire.QueueResult) recovery.Result {
	return recovery.Result{Actor: recovery.ActorRecovery, Via: recovery.ViaServer, Refused: r.Refused, Reason: r.Reason, Message: r.Message}
}
