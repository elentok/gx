package ralphloop

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/herdr"
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
}

// IterationWorktree is what PrepareIteration made for one ticket.
type IterationWorktree struct {
	Path            string
	Branch          string
	Label           string
	base            string
	featureWorktree string
	worktreeDir     string
	// keep marks a plain directory (a commitless scratch one-off) that is not a
	// git worktree and so is never removed when the ticket ends.
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
	return finishCleanup(d, &worktreeLock, o.RepoDir, w.featureWorktree, w.Path, w.Branch, "", true)
}

// FinishIteration runs a finished agent through the shared finish path: park
// on a needs-answer or zero-commit finish, otherwise land the commits onto the
// feature branch, mark the ticket done and clean up. A deferred land (lock
// held) is an error, like any failed finish; the caller parks it.
func FinishIteration(d Deps, o OneIteration, w IterationWorktree, pane, tab string) (FinishOutcome, error) {
	p := iterationParams{
		WorkspaceID:     o.WorkspaceID,
		RepoDir:         o.RepoDir,
		WorktreeDir:     w.worktreeDir,
		FeatureWorktree: w.featureWorktree,
		FeatureBranch:   o.Epic,
		Agent:           o.Agent,
		Skill:           defaultWorkerSkill,
		Ticket:          o.Ticket,
		ScratchDir:      o.ScratchDir,
		WorktreeLock:    &worktreeLock,
		SmartZone:       defaultSmartZone,
		Gate:            NewGate(),
		Sink:            noopEventSink{},
	}
	sessionID := recordLiveSession(d, w.Label, o.Ticket.Path)
	err := finishIteration(d, p, w.Path, pane, tab, w.base, w.Branch, sessionID)
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
	return FinishOutcome{Status: t.Status, Kind: events.Kind(t.ParkKind)}, nil
}

// recordLiveSession reads the native session of the agent still running under
// label and appends it to the ticket's session_ids once, as the in-process
// loop does when it launches or reattaches an iteration. The server launches
// without that step, so without this the finish path has no session to read
// the closing occupancy, elapsed time and cost from and writes none of them.
// Best-effort: "" (no live agent, no session yet) leaves the ticket as it was.
func recordLiveSession(d Deps, label, ticketPath string) string {
	agent, err := d.AgentGet(label)
	if err != nil || agent.AgentSession == "" {
		return ""
	}
	if t, err := schema.ParseTicket(ticketPath); err == nil && !slices.Contains(t.SessionIDs, agent.AgentSession) {
		_ = AppendSessionID(ticketPath, agent.AgentSession)
	}
	return agent.AgentSession
}

// landDeferCap bounds how long landBuilt waits out a held land lock before it
// gives up and lets the caller park the ticket.
const landDeferCap = 30 * time.Minute

// WaitIterationFinished blocks until the agent in pane has really finished its
// turn, the way the in-process loop does: a first idle/done is debounced, and
// an outstanding backgrounded shell command in the transcript keeps holding.
// Without this a long test run in the background reads as a finish, and the
// ticket is parked zero-commit before the agent commits.
func WaitIterationFinished(d Deps, o OneIteration, w IterationWorktree, pane string) error {
	until := []string{"idle", "done"}
	p := launchAndPromptParams{Label: w.Label, Agent: o.Agent, Pane: pane, SessionCwd: w.Path, Sink: noopEventSink{}}
	elapsedMs := 0
	for {
		if _, err := d.AgentWait(herdr.AgentWaitOptions{Target: pane, Until: until}); err != nil {
			return err
		}
		confirmed, err := confirmFinished(d, pane, until)
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
		if confirmed {
			return nil
		}
	}
}

// landBuilt lands a built iteration that was waiting on the land queue.
func landBuilt(d Deps, p iterationParams, built *builtAwaitingLandError) error {
	lp := landQueueParams{
		WorkspaceID: p.WorkspaceID, RepoDir: p.RepoDir, WorktreeDir: p.WorktreeDir,
		FeatureWorktree: p.FeatureWorktree, FeatureBranch: p.FeatureBranch, Agent: p.Agent,
		Skill: p.Skill, ScratchDir: p.ScratchDir, WorktreeLock: p.WorktreeLock,
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
	}
	return nil
}
