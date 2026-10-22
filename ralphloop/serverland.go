package ralphloop

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// FinishOutcome is how a finished iteration ended without an error: the ticket
// landed, or it parked with a status and kind. A failed finish is the error
// return of FinishIteration, never an outcome.
type FinishOutcome struct {
	Landed bool
	Status schema.Status // the park status; empty when Landed
	Kind   events.Kind
	// Reason is the park's own reason, as its run-log event recorded it.
	Reason string
}

// OneIteration is the fixed input of a single-ticket iteration driven from
// outside Run (the server's runner). Label, branch and worktree names come out
// of the same helpers Run uses, so both orchestrators leave identical state.
type OneIteration struct {
	RepoDir     string
	WorkspaceID string
	Epic        string
	ScratchDir  string
	Agent       AgentKind
	Ticket      tickets.Ticket
	// RootBase is the ref the epic's feature branch is created from when it
	// does not exist yet; "" starts it at the repo's HEAD.
	RootBase string
	// LeafBase is the identifier of the unlanded sibling whose iteration branch
	// the ticket starts from; "" starts it at the feature tip.
	LeafBase string
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
	Sink EventSink
	// StallNudgeDelay is how long an idle agent with no question and no finished
	// report waits before the one stall nudge; zero sends none.
	StallNudgeDelay time.Duration
}

// StallNudgeText is the fixed text of the stall nudge.
const StallNudgeText = "If you are waiting for a person's reply, run `gx tickets ask` now. Otherwise continue."

func (o OneIteration) sink() EventSink {
	if o.Sink == nil {
		return noopEventSink{}
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

// worktreeLock serializes git worktree add/remove per process, as Run's
// worktreeMu does per run.
var worktreeLock sync.Mutex

// PrepareIteration creates the epic's feature worktree and the ticket's
// iteration worktree off the feature tip.
func PrepareIteration(d Deps, o OneIteration) (IterationWorktree, error) {
	wtDir, err := d.WorktreeDir(o.RepoDir)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving worktree directory for %q: %w", o.RepoDir, err)
	}
	featurePath := filepath.Join(wtDir, o.Epic)
	worktreeLock.Lock()
	defer worktreeLock.Unlock()
	if err := d.AddWorktree(o.RepoDir, featurePath, o.Epic, o.RootBase); err != nil {
		return IterationWorktree{}, fmt.Errorf("creating feature worktree for branch %q: %w", o.Epic, err)
	}
	branch := iterBranch(o.Epic, o.Ticket.Identifier)
	baseRef := o.Epic
	if o.LeafBase != "" {
		baseRef = iterBranch(o.Epic, o.LeafBase)
	}
	base, err := d.RevParse(featurePath, baseRef)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving %s tip: %w", baseRef, err)
	}
	path := iterationWorktreePath(wtDir, o.Epic, o.Ticket.Identifier)
	if err := d.AddWorktree(o.RepoDir, path, branch, base); err != nil {
		return IterationWorktree{}, fmt.Errorf("creating iteration worktree: %w", err)
	}
	return IterationWorktree{
		Path: path, Branch: branch, Label: iterLabel(o.Epic, o.Ticket.Identifier),
		base: base, featureWorktree: featurePath, worktreeDir: wtDir,
	}, nil
}

// Base is the feature tip the iteration branched from. A server that restarts
// mid-iteration persists it and hands it back to ResumeIteration.
func (w IterationWorktree) Base() string { return w.base }

// ResumeIteration rebuilds the handle PrepareIteration made for a ticket whose
// worktree already exists, without touching git.
func ResumeIteration(d Deps, o OneIteration, base string) (IterationWorktree, error) {
	wtDir, err := d.WorktreeDir(o.RepoDir)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving worktree directory for %q: %w", o.RepoDir, err)
	}
	return IterationWorktree{
		Path: iterationWorktreePath(wtDir, o.Epic, o.Ticket.Identifier), Branch: iterBranch(o.Epic, o.Ticket.Identifier),
		Label: iterLabel(o.Epic, o.Ticket.Identifier),
		base:  base, featureWorktree: filepath.Join(wtDir, o.Epic), worktreeDir: wtDir,
	}, nil
}

// DiscardIteration removes a prepared iteration's worktree and branch, for a
// launch that failed before the agent produced anything.
func DiscardIteration(d Deps, o OneIteration, w IterationWorktree) error {
	return finishCleanup(d, &worktreeLock, o.RepoDir, w.featureWorktree, w.Path, w.Branch, agentrunner.Session{}, true)
}

// FinishIteration runs a finished agent through the shared finish path: park
// on a needs-answer or zero-commit finish, otherwise land the commits onto the
// feature branch, mark the ticket done and clean up. A deferred land (lock
// held) is an error, like any failed finish; the caller parks it.
func FinishIteration(d Deps, o OneIteration, w IterationWorktree, pane string) (FinishOutcome, error) {
	p := iterationParams{
		WorkspaceID:     o.WorkspaceID,
		RepoDir:         o.RepoDir,
		WorktreeDir:     w.worktreeDir,
		FeatureWorktree: w.featureWorktree,
		FeatureBranch:   o.Epic,
		Agent:           o.Agent,
		Runner:          runnerName(d),
		Label:           w.Label,
		Model:           o.Model,
		Effort:          o.Effort,
		Skill:           defaultWorkerSkill,
		KeepAskedAgent:  o.KeepAskedAgent,
		Ticket:          o.Ticket,
		ScratchDir:      o.ScratchDir,
		WorktreeLock:    &worktreeLock,
		SmartZone:       defaultSmartZone,
		Gate:            NewGate(),
		Sink:            o.sink(),
	}
	sessionID := recordLiveSession(d, w.Label, o.Ticket.Path)
	err := finishIteration(d, p, w.Path, pane, w.base, w.Branch, sessionID)
	var built *builtAwaitingLandError
	if err != nil && !errors.As(err, &built) {
		return FinishOutcome{}, err
	}
	if built != nil {
		if err := landBuilt(d, p, built); err != nil {
			return FinishOutcome{}, err
		}
	}
	t, err := schema.ParseTicket(o.Ticket.Path)
	if err != nil {
		return FinishOutcome{}, fmt.Errorf("reading finished ticket: %w", err)
	}
	if t.Status == schema.StatusDone {
		return FinishOutcome{Landed: true}, nil
	}
	// A ticket that neither landed nor parked is stuck with nobody told why;
	// fail the finish so the caller parks it with a reason.
	if t.PausedClass() == "" {
		return FinishOutcome{}, fmt.Errorf("iteration ended with the ticket %s and no park recorded", t.Status)
	}
	return parkedOutcome(o, t), nil
}

// parkedOutcome is the outcome of a finish that left t parked. Kind is never
// blank and Reason never empty, so a person is always told something true.
func parkedOutcome(o OneIteration, t schema.Ticket) FinishOutcome {
	kind := events.Kind(t.PauseReason)
	if kind == "" {
		kind = events.IterationError
	}
	reason := latestParkReason(o.ScratchDir, o.Epic, o.Ticket.Identifier)
	if reason == "" {
		reason = "the run log had no park reason; see " + RunLogPath(o.ScratchDir, o.Epic)
	}
	return FinishOutcome{Status: t.PausedClass().LegacyStatus(), Kind: kind, Reason: reason}
}

// latestParkReason is the reason of the ticket's most recent park event, or ""
// when the run log has none.
func latestParkReason(scratchDir, epic, ticket string) string {
	ev, _ := LatestParkEvent(scratchDir, epic, ticket)
	return ev.Reason
}

// LatestParkEvent is the ticket's most recent park event; false when the run
// log has none.
func LatestParkEvent(scratchDir, epic, ticket string) (Event, bool) {
	evs, ok, err := ReadEvents(scratchDir, epic)
	if err != nil || !ok {
		return Event{}, false
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Ticket == ticket && isParkType(events.Type(evs[i].Type)) {
			return evs[i], true
		}
	}
	return Event{}, false
}

// LatestLandedSHA is the SHA of the ticket's most recent cherry-pick, "" when
// none was logged.
func LatestLandedSHA(scratchDir, epic, ticket string) string {
	evs, _, err := ReadEvents(scratchDir, epic)
	if err != nil {
		return ""
	}
	return latestCherryPickedSHA(evs, ticket)
}

// runnerName names d's runner, "" when it has none.
func runnerName(d Deps) string {
	if d.Runner == nil {
		return ""
	}
	return d.Runner.Name()
}

// recordLiveSession reads the native session of the agent still running under
// label and appends it to the ticket's session_ids once, as the in-process
// loop does when it launches or reattaches an iteration. The server launches
// without that step, so without this the finish path has no session to read
// the closing occupancy, elapsed time and cost from and writes none of them.
// Best-effort: "" (no live agent, no session yet) leaves the ticket as it was.
func recordLiveSession(d Deps, label, ticketPath string) string {
	s, ok, err := d.Runner.Find(label)
	if err != nil || !ok {
		return ""
	}
	status, err := d.Runner.Status(s)
	if err != nil || status.SessionID == "" {
		return ""
	}
	if t, err := schema.ParseTicket(ticketPath); err == nil && !slices.Contains(t.SessionIDs, status.SessionID) {
		_ = AppendSessionID(ticketPath, status.SessionID)
	}
	return status.SessionID
}

// landDeferCap bounds how long landBuilt waits out a held land lock before it
// gives up and lets the caller park the ticket. It outlasts a whole conflict
// resolution (conflictResolutionTimeoutMs) by the lock holder, plus its own.
const landDeferCap = 2*conflictResolutionTimeoutMs*time.Millisecond + 10*time.Minute

// WaitIterationFinished blocks until the agent in session s has really
// finished its turn, the way the in-process loop does: a first idle/done is
// debounced, and an outstanding backgrounded shell command in the transcript
// keeps holding. Without this a long test run in the background reads as a
// finish, and the ticket is parked zero-commit before the agent commits.
func WaitIterationFinished(d Deps, o OneIteration, w IterationWorktree, s agentrunner.Session) error {
	until := runnerFinishStates
	p := launchAndPromptParams{Label: w.Label, Agent: o.Agent, Session: s, Pane: s.ID, SessionCwd: w.Path, Sink: o.sink()}
	elapsedMs := 0
	nudged := false
	for {
		if _, err := d.Runner.Wait(s, until, smartZonePollMs*time.Millisecond); err != nil {
			if errors.Is(err, agentrunner.ErrTimeout) {
				continue
			}
			return err
		}
		confirmed, err := confirmFinished(d, s, until)
		if err != nil {
			return fmt.Errorf("confirming %s finished: %w", w.Label, err)
		}
		if !confirmed {
			continue
		}
		sessionID := recordLiveSession(d, w.Label, o.Ticket.Path)
		confirmed, err = waitForBackgroundTasks(d, p, sessionID, until, &elapsedMs)
		if err != nil {
			return err
		}
		if !confirmed {
			continue
		}
		if nudged || o.StallNudgeDelay <= 0 {
			return nil
		}
		nudgedNow, again, err := nudgeIfStalled(d, o, w, s, until)
		if err != nil {
			return err
		}
		if !again {
			return nil
		}
		nudged = nudged || nudgedNow
	}
}

// nudgeIfStalled waits out the stall delay, then types the stall nudge into an
// agent that is still idle with no open question and no finished report.
// again tells the caller to wait for the agent to settle once more: after the
// nudge, or because the agent went back to work during the delay.
func nudgeIfStalled(d Deps, o OneIteration, w IterationWorktree, s agentrunner.Session, until []agentrunner.State) (nudged, again bool, err error) {
	if reported, err := agentReported(o.Ticket.Path); err != nil || reported {
		return false, false, err
	}
	d.Sleep(o.StallNudgeDelay)
	// The agent may have asked or finished during the delay.
	if reported, err := agentReported(o.Ticket.Path); err != nil || reported {
		return false, false, err
	}
	if _, err := d.Runner.Wait(s, until, finishConfirmMs*time.Millisecond); err != nil {
		if errors.Is(err, agentrunner.ErrTimeout) {
			return false, true, nil
		}
		return false, false, err
	}
	if err := d.Runner.Prompt(s, StallNudgeText); err != nil {
		return false, false, fmt.Errorf("stall nudge for %s: %w", w.Label, err)
	}
	err = logEvent(o.ScratchDir, o.Epic, Event{
		Type: string(events.StallNudged), Ticket: o.Ticket.Identifier, Label: w.Label, Text: StallNudgeText,
	})
	return true, true, err
}

// agentReported reports whether the ticket carries an agent report: a question
// (needs-answer) or a finished report.
func agentReported(ticketPath string) (bool, error) {
	t, err := schema.ParseTicket(ticketPath)
	if err != nil {
		return false, fmt.Errorf("reading ticket for stall check: %w", err)
	}
	return t.IterationStatus == schema.IterationStatusFinished || t.IterationStatus == schema.IterationStatusAsking, nil
}

// landBuilt lands a built iteration that was waiting on the land queue.
func landBuilt(d Deps, p iterationParams, built *builtAwaitingLandError) error {
	lp := landQueueParams{
		WorkspaceID: p.WorkspaceID, RepoDir: p.RepoDir, WorktreeDir: p.WorktreeDir,
		FeatureWorktree: p.FeatureWorktree, FeatureBranch: p.FeatureBranch, Agent: p.Agent,
		Model: p.Model, Effort: p.Effort, Skill: p.Skill, ScratchDir: p.ScratchDir, WorktreeLock: p.WorktreeLock,
		SmartZone: p.SmartZone, Gate: p.Gate, Sink: p.Sink,
	}
	r := landOne(d, lp, built.job)
	// A held land lock is routine (another landing, a person's conflict
	// resolution), so retry as runLandQueue does instead of failing the finish.
	logged := false
	for waited := time.Duration(0); r.landDeferred && waited < landDeferCap; waited += landDeferRetryInterval {
		if !logged {
			logged = true
			logLandDeferred(p)
		}
		d.Sleep(landDeferRetryInterval)
		r = landOne(d, lp, built.job)
	}
	switch {
	case r.err != nil:
		return r.err
	case r.landDeferred:
		return errors.New("land deferred: land lock held")
	case r.parkedOnChild:
		// The child's own park says why it failed; the parent would otherwise
		// stay claimed with nobody told. Its iteration branch is durable, so a
		// person (or recovery) can land it again.
		reason := "land conflict not resolved: " + r.childReason +
			"\nThe cherry-pick was rolled back; the iteration branch " + built.job.branch +
			" is intact. Merge it again with `gx server tickets merge`."
		_, err := park(p.Sink, parkRequest{
			ScratchDir: p.ScratchDir, EpicName: p.FeatureBranch, Ticket: p.Ticket.Identifier, Path: p.Ticket.Path,
			Type: events.Paused, Kind: events.MergeConflict, Reason: reason,
			Repair: schema.FaultState{Label: iterLabel(p.FeatureBranch, p.Ticket.Identifier), Branch: built.job.branch, Worktree: built.job.path},
			Event:  p.agentEvent(built.job.pane, built.job.sessionID, built.job.path),
		})
		return err
	}
	return nil
}
