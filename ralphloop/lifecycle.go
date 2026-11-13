package ralphloop

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/supervise"
	"github.com/elentok/gx/ralphloop/supervise/startprompt"
	"github.com/elentok/gx/ralphloop/ticketops"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// ticketAddress is the canonical "project:epic/NN" address of the ticket
// file at ticketPath (<root>/<epic>/issues/NN-<slug>.md), the only form
// iteration prompts hand to agents.
func ticketAddress(t tickets.Ticket) string {
	a, _ := tickets.AddressOfPath(t.Path, t.Identifier)
	return a.String()
}

// FinishOutcome is how a finished iteration ended without an error: the ticket
// merged, or it paused with a status and kind. A failed finish is the error
// return of FinishIteration, never an outcome.
type FinishOutcome struct {
	Merged bool
	// Cancelled is a commitless attempt closed as cancelled by a finish check:
	// neither merged nor paused. Reason is why.
	Cancelled bool
	Status    schema.Status // the pause status; empty when Merged
	Kind      events.Kind
	// Reason is the pause's own reason, as its run-log event recorded it.
	Reason string
}

// OneIteration is the fixed input of a single-ticket iteration driven from
// outside Run (the server's runner). Label, branch and worktree names come out
// of the same helpers Run uses, so both orchestrators leave identical state.
type OneIteration struct {
	WritableDirectories []string

	RepoDir     string
	WorkspaceID string
	Epic        string
	ScratchDir  string
	Agent       agent.AgentKind
	Ticket      tickets.Ticket
	// RootBase is the ref the epic's feature branch is created from when it
	// does not exist yet; "" starts it at the repo's HEAD.
	RootBase string
	// LeafBase is the identifier of the unmerged sibling whose iteration branch
	// the ticket starts from; "" starts it at the feature tip.
	LeafBase string
	// TargetTip is a merge ticket's merge target SHA at claim; "" for any other ticket.
	TargetTip string
	// Model and Effort are what the ticket's conflict-resolution agent starts
	// under; empty leaves the agent's own default.
	Model  string
	Effort string
	// ResumeSessionID is the session the server's launch asks a runner to
	// continue; "" starts the agent cold.
	ResumeSessionID string
	// KeepAskedAgent: see iterationParams.KeepAskedAgent.
	KeepAskedAgent bool
	// Sink hears the finish path's events; nil hears nothing.
	Sink runlog.EventSink
	// StallNudgeDelay is how long an idle agent with no question and no finished
	// report waits before the one stall nudge; zero sends none.
	StallNudgeDelay time.Duration
	// Runner is the name of the runner the agent runs on (agentrunner.Choice).
	// It picks how a block pauses: see PausesAtOnce.
	Runner string
	// SmartZone is the context-token ceiling the iteration's agent is held to,
	// the same as the SmartZone run option; zero means agent.DefaultSmartZone.
	SmartZone int
	// OnRateLimit hears the wait loop's rate-limit hold: (true, resetAt) when it
	// starts or the reset changes (zero resetAt: no known reset), (false, zero)
	// when it ends. Nil hears nothing.
	OnRateLimit func(limited bool, resetAt time.Time)
	// Context cancels the launch's dependency install; nil means
	// context.Background().
	Context context.Context
	// OnDepsStart and OnDepsDone are called around the dependency install, when
	// the ticket has one (a scratch commitless ticket has none). Nil hears nothing.
	OnDepsStart func()
	OnDepsDone  func()
	// LongTicketTurns is the assistant-turn count at which OnLongTicket is
	// called, once per launch; zero or a nil callback means no alert.
	LongTicketTurns int
	OnLongTicket    func(turns int)
}

// PausesAtOnce reports whether a block pauses the ticket with no dwell: a
// headless Claude agent's block is a tracker fact, not a pane guess. Every
// other runner and agent dwells first, since its block is read off a screen.
func (o OneIteration) PausesAtOnce() bool {
	return o.Agent == agent.AgentClaude && o.Runner == string(agentrunner.ChoiceHeadless)
}

func (o OneIteration) smartZone() int {
	if o.SmartZone <= 0 {
		return agent.DefaultSmartZone
	}
	return o.SmartZone
}

func (o OneIteration) sink() runlog.EventSink {
	if o.Sink == nil {
		return runlog.NoopEventSink{}
	}
	return o.Sink
}

// IterationWorktree is what PrepareIteration made for one ticket.
type IterationWorktree struct {
	Path            string
	Branch          string
	Label           string
	base            string
	featureWorktree string
	worktreeDir     string
	// keep marks a directory the ticket does not own (a commitless scratch
	// one-off's, or the feature worktree a merge ticket runs in), so it is never
	// removed when the ticket ends.
	keep bool
}

// PrepareIteration creates the epic's feature worktree and the ticket's
// iteration worktree off the feature tip, and installs dependencies in the
// iteration worktree.
func PrepareIteration(d deps.Deps, o OneIteration) (IterationWorktree, error) {
	w, err := addIterationWorktrees(d, o)
	if err != nil {
		return IterationWorktree{}, err
	}
	if err := installDepsIn(d, o, w.Path); err != nil {
		return IterationWorktree{}, discardAfterInstallFailure(err, DiscardIteration(d, o, w))
	}
	return w, nil
}

func addIterationWorktrees(d deps.Deps, o OneIteration) (IterationWorktree, error) {
	wtDir, err := d.WorktreeDir(o.RepoDir)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving worktree directory for %q: %w", o.RepoDir, err)
	}
	featurePath := filepath.Join(wtDir, o.Epic)
	worktree.Lock.Lock()
	defer worktree.Lock.Unlock()
	if err := addFeatureWorktree(d, o, featurePath); err != nil {
		return IterationWorktree{}, fmt.Errorf("creating feature worktree for branch %q: %w", o.Epic, err)
	}
	branch := worktree.IterationBranch(o.Epic, o.Ticket.Identifier)
	baseRef := o.Epic
	if o.LeafBase != "" {
		baseRef = worktree.IterationBranch(o.Epic, o.LeafBase)
	}
	base, err := d.RevParse(featurePath, baseRef)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving %s tip: %w", baseRef, err)
	}
	path := worktree.IterationWorktreePath(wtDir, o.Epic, o.Ticket.Identifier)
	if err := d.AddWorktree(o.RepoDir, path, branch, base); err != nil {
		return IterationWorktree{}, fmt.Errorf("creating iteration worktree: %w", err)
	}
	return IterationWorktree{
		Path: path, Branch: branch, Label: worktree.IterationLabel(o.Epic, o.Ticket.Identifier),
		base: base, featureWorktree: featurePath, worktreeDir: wtDir,
	}, nil
}

// Base is the feature tip the iteration branched from. A server that restarts
// mid-iteration persists it and hands it back to ResumeIteration.
func (w IterationWorktree) Base() string { return w.base }

// ResumeIteration rebuilds the handle PrepareIteration made for a ticket whose
// worktree already exists, without touching git.
func ResumeIteration(d deps.Deps, o OneIteration, base string) (IterationWorktree, error) {
	wtDir, err := d.WorktreeDir(o.RepoDir)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving worktree directory for %q: %w", o.RepoDir, err)
	}
	return IterationWorktree{
		Path: worktree.IterationWorktreePath(wtDir, o.Epic, o.Ticket.Identifier), Branch: worktree.IterationBranch(o.Epic, o.Ticket.Identifier),
		Label: worktree.IterationLabel(o.Epic, o.Ticket.Identifier),
		base:  base, featureWorktree: filepath.Join(wtDir, o.Epic), worktreeDir: wtDir,
	}, nil
}

// DiscardIteration removes a prepared iteration's worktree and branch, for a
// launch that failed before the agent produced anything.
func DiscardIteration(d deps.Deps, o OneIteration, w IterationWorktree) error {
	return finishCleanup(d, o.RepoDir, w.featureWorktree, w.Path, w.Branch, agentrunner.Session{}, true)
}

// FinishIteration runs a finished agent through the shared finish path: pause
// on a paused (answer) or zero-commit finish, otherwise merge the commits onto the
// feature branch, mark the ticket done and clean up. A deferred merge (lock
// held) is an error, like any failed finish; the caller pauses it.
func FinishIteration(d deps.Deps, o OneIteration, w IterationWorktree, pane string) (FinishOutcome, error) {
	p := iterationParams{
		WritableDirectories: o.WritableDirectories,
		WorkspaceID:         o.WorkspaceID,
		RepoDir:             o.RepoDir,
		WorktreeDir:         w.worktreeDir,
		FeatureWorktree:     w.featureWorktree,
		FeatureBranch:       o.Epic,
		Agent:               o.Agent,
		Runner:              runnerName(d),
		Label:               w.Label,
		Model:               o.Model,
		Effort:              o.Effort,
		Skill:               agent.DefaultWorkerSkill,
		KeepAskedAgent:      o.KeepAskedAgent,
		Ticket:              o.Ticket,
		ScratchDir:          o.ScratchDir,
		SmartZone:           o.smartZone(),
		Gate:                supervise.NewGate(),
		Sink:                o.sink(),
	}
	sessionID := supervise.RecordLiveSession(d, w.Label, o.Ticket.Path)
	err := finishIteration(d, p, w.Path, pane, w.base, w.Branch, sessionID)
	var built *builtAwaitingMergeError
	if err != nil && !errors.As(err, &built) {
		return FinishOutcome{}, err
	}
	if built != nil {
		if err := mergeBuilt(d, p, built); err != nil {
			return FinishOutcome{}, err
		}
	}
	t, err := schema.ParseTicket(o.Ticket.Path)
	if err != nil {
		return FinishOutcome{}, fmt.Errorf("reading finished ticket: %w", err)
	}
	if t.Status == schema.StatusDone {
		return FinishOutcome{Merged: true}, nil
	}
	// A ticket that neither merged nor paused is stuck with nobody told why;
	// fail the finish so the caller pauses it with a reason.
	if t.PausedClass() == "" {
		return FinishOutcome{}, fmt.Errorf("iteration ended with the ticket %s and no pause recorded", t.Status)
	}
	return pausedOutcome(o, t), nil
}

// pausedOutcome is the outcome of a finish that left t paused. Kind is never
// blank and Reason never empty, so a person is always told something true.
func pausedOutcome(o OneIteration, t schema.Ticket) FinishOutcome {
	kind := events.Kind(t.PauseReason)
	if kind == "" {
		kind = events.IterationError
	}
	reason := latestPauseReason(o.ScratchDir, o.Epic, o.Ticket.Identifier)
	if reason == "" {
		reason = "the run log had no pause reason; see " + runlog.RunLogPath(o.ScratchDir, o.Epic)
	}
	return FinishOutcome{Status: t.PausedClass().LegacyStatus(), Kind: kind, Reason: reason}
}

// latestPauseReason is the reason of the ticket's most recent pause event, or ""
// when the run log has none.
func latestPauseReason(scratchDir, epic, ticket string) string {
	ev, _ := runlog.LatestPauseEvent(scratchDir, epic, ticket)
	return ev.Reason
}

// runnerName names d's runner, "" when it has none.
func runnerName(d deps.Deps) string {
	if d.Runner == nil {
		return ""
	}
	return d.Runner.Name()
}

// mergeDeferCap bounds how long mergeBuilt waits out a held merge lock before it
// gives up and lets the caller pause the ticket. It outlasts a whole conflict
// resolution (conflictResolutionTimeoutMs) by the lock holder, plus its own.
const mergeDeferCap = 2*conflictResolutionTimeoutMs*time.Millisecond + 10*time.Minute

// WaitIterationFinished blocks until the agent in session s has really
// finished its turn, the way the in-process loop does: a first idle/done is
// debounced, and an outstanding backgrounded shell command in the transcript
// keeps holding. Without this a long test run in the background reads as a
// finish, and the ticket is paused zero-commit before the agent commits.
func WaitIterationFinished(d deps.Deps, o OneIteration, w IterationWorktree, s agentrunner.Session) error {
	until := supervise.RunnerFinishStates
	waitStates := append(slices.Clone(until), agentrunner.StateBlocked)
	p := supervise.LaunchParams{
		Label: w.Label, Agent: o.Agent, Session: s, Pane: s.ID, SessionCwd: w.Path, Sink: o.sink(),
		ScratchDir: o.ScratchDir, EpicName: o.Epic, Ticket: o.Ticket.Identifier, TicketPath: o.Ticket.Path,
		TicketData: o.Ticket, SmartZone: o.smartZone(), VerifyCap: new(supervise.VerifyCapState), Model: o.Model,
		LongTicketTurns: o.LongTicketTurns, OnLongTicket: o.OnLongTicket,
	}
	in := supervise.WatchPersonInput(d.Runner, s)
	defer in.Close()
	p.Input = in
	vcap := supervise.NewVerifyCap(p, p.SmartZone)
	now := supervise.ClockOf(d)
	mguard := supervise.NewModelGuard(d, p, now)
	lguard := supervise.NewLongTicketGuard(d, p, now)
	elapsedMs := 0
	nudged := false
	outageResends := 0
	var codexHold supervise.CodexHold
	for {
		poll := cmp.Or(d.GuardPoll, supervise.SmartZonePollMs*time.Millisecond)
		st, err := d.Runner.Wait(s, waitStates, poll)
		if err != nil {
			if errors.Is(err, agentrunner.ErrTimeout) {
				sessionID := ""
				if status, err := d.Runner.Status(s); err == nil {
					sessionID = status.SessionID
				}
				if err := codexHold.Tick(d, p, sessionID, int(poll/time.Millisecond)); err != nil {
					return err
				}
				if err := mguard.Tick(d, p, sessionID); err != nil {
					return err
				}
				lguard.Tick(d, p, sessionID)
				if _, err := vcap.Tick(d, p, sessionID); err != nil {
					return err
				}
				continue
			}
			return err
		}
		// A limit holds the agent in place before any blocked pause or finish check.
		if resetAt, limited, _, _ := supervise.RunnerRateLimit(d, p); limited {
			if err := supervise.HoldRateLimit(d, o.OnRateLimit, p, waitStates, resetAt); err != nil {
				return err
			}
			continue
		}
		if st.State == agentrunner.StateBlocked {
			if o.PausesAtOnce() {
				return supervise.PauseBlockedAtOnce(d, p, o.Ticket.Path, st)
			}
			reason, paused, err := supervise.DwellThenPauseBlocked(d, p, o.Runner, supervise.RecordLiveSession(d, w.Label, o.Ticket.Path))
			if err != nil {
				return err
			}
			if paused {
				return &supervise.BlockedPausedError{Reason: reason}
			}
			continue
		}
		// A classifier outage ends the turn without the agent choosing to: re-send
		// "continue" at growing delays instead of reading a finish.
		outage := classifierOutage(d, p, s)
		if !outage {
			outageResends = 0
		} else if outageResends < len(classifierResendDelays) {
			in.Sleep(d, classifierResendDelay(outageResends))
			outageResends++
			if classifierOutage(d, p, s) {
				if err := d.Runner.Prompt(s, classifierResendText); err != nil && !errors.Is(err, agentrunner.ErrNotReady) {
					return fmt.Errorf("re-sending continue to %s: %w", w.Label, err)
				}
			}
			continue
		} else {
			return pauseClassifierOutage(d, p, w, o)
		}
		confirmed, err := supervise.ConfirmFinished(d, in, s, until)
		if err != nil {
			return fmt.Errorf("confirming %s finished: %w", w.Label, err)
		}
		if !confirmed {
			continue
		}
		sessionID := supervise.RecordLiveSession(d, w.Label, o.Ticket.Path)
		confirmed, err = supervise.WaitForBackgroundTasks(d, p, sessionID, until, &elapsedMs)
		if err != nil {
			return err
		}
		if !confirmed {
			continue
		}
		// The nudge tells the agent to run gx, which an outage fails too.
		if nudged || o.StallNudgeDelay <= 0 || outage {
			return nil
		}
		nudgedNow, again, err := nudgeIfStalled(d, in, o, w, s, until, p, sessionID)
		if err != nil {
			return err
		}
		if !again {
			return nil
		}
		nudged = nudged || nudgedNow
	}
}

// pauseClassifierOutage pauses the ticket once the re-sends are used up and
// the classifier still gives no verdict.
func pauseClassifierOutage(d deps.Deps, p supervise.LaunchParams, w IterationWorktree, o OneIteration) error {
	reason := fmt.Sprintf("%s: the classifier gave no verdict after %d re-sends of %q", w.Label, len(classifierResendDelays), classifierResendText)
	ticketops.PauseTicket(p.EventSink(), ticketops.PauseRequest{
		ScratchDir: p.ScratchDir, EpicName: p.EpicName, Ticket: p.Ticket, Path: p.TicketPath,
		Type: events.Paused, Kind: events.ClassifierOutage, Reason: reason,
		Event: runlog.Event{Agent: p.Agent, Pane: p.Pane, AgentSession: supervise.RecordLiveSession(d, w.Label, o.Ticket.Path), Cwd: p.SessionCwd},
	})
	return &supervise.BlockedPausedError{Reason: reason}
}

// classifierResendDelays are the waits before each "continue" re-send during a
// classifier outage; after the last the ticket pauses classifier-outage.
var classifierResendDelays = []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute}

// ClassifierResendDelayEnv replaces every re-send delay with one Go duration:
// the fixture's knob, since seven minutes is out of reach of a browser flow.
const ClassifierResendDelayEnv = "GX_CLASSIFIER_RESEND_DELAY"

// classifierResendDelay is the wait before re-send number i (from zero).
func classifierResendDelay(i int) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(ClassifierResendDelayEnv)); err == nil && d >= 0 {
		return d
	}
	return classifierResendDelays[i]
}

const classifierResendText = "continue"

// classifierOutage reports whether a Claude transcript ends in "no verdict"
// tool errors. Codex has no auto-mode classifier.
func classifierOutage(d deps.Deps, p supervise.LaunchParams, s agentrunner.Session) bool {
	if p.Agent != agent.AgentClaude || d.ReadClassifierOutage == nil {
		return false
	}
	status, err := d.Runner.Status(s)
	if err != nil || status.SessionID == "" {
		return false
	}
	n, err := d.ReadClassifierOutage(p.SessionCwd, status.SessionID)
	return err == nil && n > 0
}

// RelaunchRequest is the wait's answer to a pane that stopped taking keys: the
// caller stops the session and launches a fresh one. Err is the hung-pane error.
type RelaunchRequest struct{ Err error }

func (e *RelaunchRequest) Error() string { return e.Err.Error() }
func (e *RelaunchRequest) Unwrap() error { return e.Err }

// nudgeIfStalled waits out the stall delay, then types the stall nudge into an
// agent that is still idle with no open question and no finished report.
// again tells the caller to wait for the agent to settle once more: after the
// nudge, or because the agent went back to work during the delay.
func nudgeIfStalled(d deps.Deps, in *supervise.PersonInput, o OneIteration, w IterationWorktree, s agentrunner.Session, until []agentrunner.State, p supervise.LaunchParams, sessionID string) (nudged, again bool, err error) {
	if reported, err := agentReported(o.Ticket.Path); err != nil || reported {
		return false, false, err
	}
	in.Sleep(d, o.StallNudgeDelay)
	// The agent may have asked or finished during the delay.
	if reported, err := agentReported(o.Ticket.Path); err != nil || reported {
		return false, false, err
	}
	if _, err := d.Runner.Wait(s, until, supervise.FinishConfirmMs*time.Millisecond); err != nil {
		if errors.Is(err, agentrunner.ErrTimeout) {
			return false, true, nil
		}
		return false, false, err
	}
	// A task started during the delay is live: back to the finish wait, which
	// holds on it, instead of typing into an agent that is not stalled.
	if supervise.BackgroundTaskLive(d, p, sessionID) {
		return false, true, nil
	}
	text := stallNudgeText(d, o, w, s)
	err = d.Runner.Prompt(s, text)
	// A hung pane matches ErrInputNotEmpty too, but a retry types into the
	// same dead pane: only a fresh session helps.
	if _, hung := errors.AsType[*claudeui.HungPaneError](err); hung {
		return false, false, &RelaunchRequest{Err: err}
	}
	// A not-delivered prompt started no turn, so one retry cannot double-send.
	if errors.Is(err, agentrunner.ErrNotDelivered) && errors.Is(err, claudeui.ErrInputNotEmpty) {
		_ = runlog.AppendEvent(o.ScratchDir, o.Epic, runlog.Event{
			Type: string(events.PromptNudged), Ticket: o.Ticket.Identifier, Label: w.Label, Attempt: 1,
			Reason: "stall nudge retried: " + err.Error(),
		})
		err = d.Runner.Prompt(s, text)
	}
	if err != nil {
		// A refusal means the agent went back to work after the wait: skip the nudge.
		if errors.Is(err, agentrunner.ErrNotReady) {
			if st, statusErr := d.Runner.Status(s); statusErr == nil && st.State == agentrunner.StateWorking {
				return false, true, nil
			}
		}
		return false, false, fmt.Errorf("stall nudge for %s: %w", w.Label, err)
	}
	err = runlog.AppendEvent(o.ScratchDir, o.Epic, runlog.Event{
		Type: string(events.StallNudged), Ticket: o.Ticket.Identifier, Label: w.Label, Text: text,
	})
	return true, true, err
}

// stallNudgeText is the launch prompt again for a session whose first person
// record is not the launch prompt (it never got its task), else the generic
// nudge. A Codex session with no rollout never got its task either, so it is
// never given the generic nudge; for Claude an unreadable or missing
// transcript keeps the generic text.
func stallNudgeText(d deps.Deps, o OneIteration, w IterationWorktree, s agentrunner.Session) string {
	if d.ReadLaunchRecorded == nil {
		return startprompt.StallNudgeText
	}
	status, err := d.Runner.Status(s)
	if err != nil || status.SessionID == "" {
		return startprompt.StallNudgeText
	}
	launch := agent.SkillPrompt(o.Agent, agent.LaunchSkill(o.Ticket, agent.DefaultWorkerSkill), ticketAddress(o.Ticket))
	recorded, known, err := d.ReadLaunchRecorded(o.Agent, w.Path, status.SessionID, launch)
	if err != nil || recorded || (!known && o.Agent != agent.AgentCodex) {
		return startprompt.StallNudgeText
	}
	return launch
}

// agentReported reports whether the ticket carries an agent report: a question
// (paused (answer)) or a finished report.
func agentReported(ticketPath string) (bool, error) {
	t, err := schema.ParseTicket(ticketPath)
	if err != nil {
		return false, fmt.Errorf("reading ticket for stall check: %w", err)
	}
	return t.IterationStatus == schema.IterationStatusFinished || t.IterationStatus == schema.IterationStatusAsking, nil
}

// mergeBuilt merges a built iteration that was waiting on the merge queue.
func mergeBuilt(d deps.Deps, p iterationParams, built *builtAwaitingMergeError) error {
	lp := mergeQueueParams{
		WritableDirectories: p.WritableDirectories,
		WorkspaceID:         p.WorkspaceID, RepoDir: p.RepoDir, WorktreeDir: p.WorktreeDir,
		FeatureWorktree: p.FeatureWorktree, FeatureBranch: p.FeatureBranch, Agent: p.Agent,
		Model: p.Model, Effort: p.Effort, Skill: p.Skill, ScratchDir: p.ScratchDir,
		SmartZone: p.SmartZone, Gate: p.Gate, Sink: p.Sink,
	}
	r := mergeOne(d, lp, built.job)
	// A held merge lock is routine (another merging, a person's conflict
	// resolution), so retry instead of failing the finish.
	logged := false
	for waited := time.Duration(0); r.mergeDeferred && waited < mergeDeferCap; waited += mergeDeferRetryInterval {
		if !logged {
			logged = true
			logMergeDeferred(d, p)
		}
		d.Sleep(mergeDeferRetryInterval)
		r = mergeOne(d, lp, built.job)
	}
	switch {
	case r.err != nil:
		return r.err
	case r.mergeDeferred:
		return errors.New("merge deferred: merge lock held")
	case r.pausedOnChild:
		// The child's own pause says why it failed; the parent would otherwise
		// stay claimed with nobody told. Its iteration branch is durable, so a
		// person (or recovery) can merge it again.
		reason := "merge conflict not resolved: " + r.childReason +
			"\nThe cherry-pick was rolled back; the iteration branch " + built.job.branch +
			" is intact. Merge it again with `gx server tickets merge`."
		_, err := ticketops.PauseTicket(p.Sink, ticketops.PauseRequest{
			ScratchDir: p.ScratchDir, EpicName: p.FeatureBranch, Ticket: p.Ticket.Identifier, Path: p.Ticket.Path,
			Type: events.Paused, Kind: events.MergeConflict, Reason: reason,
			Repair: schema.FaultState{Label: worktree.IterationLabel(p.FeatureBranch, p.Ticket.Identifier), Branch: built.job.branch, Worktree: built.job.path},
			Event:  p.agentEvent(built.job.pane, built.job.sessionID, built.job.path),
		})
		return err
	}
	return nil
}
