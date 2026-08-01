package ralphloop

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// defaultScratchDir is the ticket tracker directory used when
// RunOptions.ScratchDir (or Resume's scratchDir) is unset.
const defaultScratchDir = ".scratch"

// defaultMaxParallel is how many iterations run concurrently when
// RunOptions.MaxParallel is unset.
const defaultMaxParallel = 2

// defaultSmartZone is the context-token ceiling used when
// RunOptions.SmartZone is unset.
const defaultSmartZone = 150_000

// smartZonePollMs bounds each "wait for the agent to finish" poll tick, so a
// running iteration's context occupancy is checked against --smart-zone at
// roughly this cadence instead of only once the agent settles.
const smartZonePollMs = 30_000

// finishDebounceMs is how long waitForFinish and finishIteration each pause
// before re-checking a just-reached "finished" signal, and finishConfirmMs is
// how long that recheck waits for the agent to prove it's still finished.
// herdr's idle/done status reflects pane output settling, not a genuine
// end-of-turn signal, so an agent that briefly stops producing output mid-turn
// (e.g. between its last tool call and a commit) can look finished for an
// instant. Without this debounce the loop would declare the iteration done,
// mark it needs-info (no commits yet), and abandon the worktree/tab while the
// agent went on to actually finish and commit — orphaning real, landed work.
const (
	finishDebounceMs = 3_000
	finishConfirmMs  = 2_000
)

// AgentKind identifies the coding agent that drives an iteration.
type AgentKind string

const (
	AgentClaude AgentKind = "claude"
	AgentCodex  AgentKind = "codex"
)

func (a AgentKind) valid() bool {
	return a == AgentClaude || a == AgentCodex
}

// ValidateAgentKind reports whether agent names a supported iteration agent.
func ValidateAgentKind(agent AgentKind) error {
	if agent == "" {
		return nil
	}
	if !agent.valid() {
		return fmt.Errorf("invalid agent %q: must be claude or codex", agent)
	}
	return nil
}

func skillPrompt(agent AgentKind, skill, ticketPath string) string {
	prefix := "/"
	if agent == AgentCodex {
		prefix = "$"
	}
	if ticketPath == "" {
		return prefix + skill
	}
	return fmt.Sprintf("%s%s %s", prefix, skill, ticketPath)
}

func agentArgs(agent AgentKind, scratchDir, epicName string) []string {
	if agent == AgentCodex {
		return []string{
			"--sandbox", "workspace-write",
			"--ask-for-approval", "on-request",
			"--add-dir", filepath.Join(scratchDir, epicName),
		}
	}
	return []string{"--permission-mode", "auto"}
}

// RunOptions configures a single `gx ralph-loop {epic-name}` invocation.
type RunOptions struct {
	EpicName    string
	Agent       AgentKind // defaults to AgentClaude
	Skill       string    // skill each iteration invokes, e.g. "implement"
	ScratchDir  string    // defaults to ".scratch"
	RepoDir     string    // repo root passed as the herdr workspace/worktree cwd
	MaxParallel int       // defaults to defaultMaxParallel; how many iterations run concurrently
	SmartZone   int       // defaults to defaultSmartZone; context-token ceiling before pausing an iteration
}

// Run drives every unblocked ticket in the named epic to completion, up to
// MaxParallel running concurrently, each in its own iteration worktree:
// create the iteration worktree, launch the selected agent, send its initial
// skill prompt, wait for it to finish, cherry-pick its
// commits onto the feature branch, mark the ticket done, and remove the
// iteration worktree. As soon as one iteration finishes, a freed slot is
// backfilled with the next frontier ticket. It exits once every ticket in
// the epic reaches a done-family status, or immediately if the epic has none
// to run.
func Run(opts RunOptions, d Deps, out io.Writer) error {
	agent := opts.Agent
	if agent == "" {
		agent = AgentClaude
	}
	if err := ValidateAgentKind(agent); err != nil {
		return err
	}

	scratchDir := opts.ScratchDir
	if scratchDir == "" {
		scratchDir = defaultScratchDir
	}
	scratchDir, err := filepath.Abs(scratchDir)
	if err != nil {
		return fmt.Errorf("resolving scratch directory: %w", err)
	}
	maxParallel := opts.MaxParallel
	if maxParallel <= 0 {
		maxParallel = defaultMaxParallel
	}
	smartZone := opts.SmartZone
	if smartZone <= 0 {
		smartZone = defaultSmartZone
	}

	initial, err := loadNamedEpic(scratchDir, opts.EpicName)
	if err != nil {
		return err
	}
	if initial == nil || len(initial.Tickets) == 0 {
		fmt.Fprintf(out, "no tickets found for epic %q; nothing to do\n", opts.EpicName)
		return nil
	}
	if allSettled(*initial) {
		fmt.Fprintf(out, "epic %q is already complete (%d/%d done)\n", opts.EpicName, initial.DoneCount(), initial.TotalCount())
		return nil
	}

	workspaceID, err := d.FindOrCreateWorkspace(opts.EpicName, opts.RepoDir)
	if err != nil {
		return fmt.Errorf("finding/creating herdr workspace %q: %w", opts.EpicName, err)
	}

	wtDir, err := d.WorktreeDir(opts.RepoDir)
	if err != nil {
		return fmt.Errorf("resolving worktree directory for %q: %w", opts.RepoDir, err)
	}

	featurePath := filepath.Join(wtDir, opts.EpicName)
	if err := d.AddWorktree(opts.RepoDir, featurePath, opts.EpicName, ""); err != nil {
		return fmt.Errorf("creating feature worktree for branch %q: %w", opts.EpicName, err)
	}

	// scheduleMu guards reading the frontier and claiming a ticket, so two
	// concurrently-running iterations never race each other onto the same
	// ticket. featureMu guards the one operation that mutates the shared
	// feature worktree's working directory (the cherry-pick landing a
	// finished iteration's commits), so concurrent iterations never cherry-
	// pick into it at the same time. outMu guards out itself, since a paused
	// iteration reports its pause/resume from its own goroutine.
	var scheduleMu sync.Mutex
	var featureMu sync.Mutex
	var outMu sync.Mutex
	report := func(format string, args ...any) {
		outMu.Lock()
		defer outMu.Unlock()
		fmt.Fprintf(out, format, args...)
	}

	gate := newPauseGate()
	resumePath := resumeSignalPath(scratchDir, opts.EpicName)

	type outcome struct {
		ticket tickets.Ticket
		err    error
	}
	results := make(chan outcome)

	// claimNext claims and returns the next frontier ticket, or ok=false if
	// none is available right now (every remaining ticket is blocked,
	// already claimed by a running iteration, the loop is paused on a
	// smart-zone breach, or the epic is done).
	claimNext := func() (ticket tickets.Ticket, ok bool, err error) {
		scheduleMu.Lock()
		defer scheduleMu.Unlock()

		if gate.isPaused() {
			return tickets.Ticket{}, false, nil
		}

		epic, err := loadNamedEpic(scratchDir, opts.EpicName)
		if err != nil {
			return tickets.Ticket{}, false, err
		}
		frontier := Frontier(*epic)
		if len(frontier) == 0 {
			return tickets.Ticket{}, false, nil
		}
		ticket = frontier[0]
		if err := Claim(ticket.Path); err != nil {
			return tickets.Ticket{}, false, fmt.Errorf("claiming ticket %s: %w", ticket.Identifier, err)
		}
		return ticket, true, nil
	}

	launch := func(ticket tickets.Ticket, reattach bool) {
		go func() {
			params := iterationParams{
				WorkspaceID:      workspaceID,
				RepoDir:          opts.RepoDir,
				WorktreeDir:      wtDir,
				FeatureWorktree:  featurePath,
				FeatureBranch:    opts.EpicName,
				Agent:            agent,
				Skill:            opts.Skill,
				Ticket:           ticket,
				ScratchDir:       scratchDir,
				FeatureLock:      &featureMu,
				SmartZone:        smartZone,
				Gate:             gate,
				ResumeSignalPath: resumePath,
				Report:           report,
			}
			var err error
			if reattach {
				err = reattachIteration(d, params)
			} else {
				err = runIteration(d, params)
			}
			results <- outcome{ticket: ticket, err: err}
		}()
	}

	completed := 0
	active := 0

	reattached, err := reconcile(d, reconcileParams{
		WorkspaceID:      workspaceID,
		Paths:            reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: featurePath, WorktreeDir: wtDir, RepoDir: opts.RepoDir},
		Agent:            agent,
		Skill:            opts.Skill,
		SmartZone:        smartZone,
		Gate:             gate,
		ResumeSignalPath: resumePath,
		FeatureLock:      &featureMu,
		Report:           report,
	}, *initial)
	if err != nil {
		return err
	}
	for _, ticket := range initial.Tickets {
		if strings.EqualFold(strings.TrimSpace(ticket.Status), "needs-attention") {
			gate.pause(iterLabel(ticket.Identifier), "needs operator attention")
		}
	}
	for _, ticket := range reattached {
		launch(ticket, true)
		active++
	}

	for {
		epic, err := loadNamedEpic(scratchDir, opts.EpicName)
		if err != nil {
			return err
		}
		if allSettled(*epic) && active == 0 {
			break
		}

		for active < maxParallel {
			ticket, ok, err := claimNext()
			if err != nil {
				return err
			}
			if !ok {
				break
			}
			launch(ticket, false)
			active++
		}

		if active == 0 {
			if gate.isPaused() {
				return fmt.Errorf("epic %q paused with no running iterations left: %v", opts.EpicName, gate.snapshot())
			}
			return fmt.Errorf("epic %q has no unblocked tickets left but isn't all done; check for a stuck ticket", opts.EpicName)
		}

		r := <-results
		active--
		if r.err != nil {
			for active > 0 {
				<-results
				active--
			}
			return fmt.Errorf("ticket %s: %w", r.ticket.Identifier, r.err)
		}

		report("ticket %s %q landed on %s\n", r.ticket.Identifier, r.ticket.Title, opts.EpicName)
		completed++
	}

	report("ralph-loop %q complete: %d ticket(s) landed on %s\n", opts.EpicName, completed, opts.EpicName)
	return nil
}

// allSettled reports whether every ticket in e has reached a terminal state
// from the loop's perspective: done, needs-info (an iteration that finished
// with no commits to land, left for inspection), or needs-attention (Codex
// operator intervention, or a done ticket startup reconciliation found
// unrecoverable — see markDoneTicketUnrecoverable). Unlike
// tickets.Epic.AllDone — which only tickets in the done family and is shared
// with the tickets UI's collapse/expand rendering — these count as terminal
// here too, so the loop can exit cleanly once every remaining ticket is
// either landed or stuck needing a human, rather than looping forever
// (Frontier already excludes both from scheduling). A needs-attention ticket
// still tied to a live, running iteration keeps active above zero until that
// iteration settles, so this doesn't race the gate-pause path in Run.
func allSettled(e tickets.Epic) bool {
	if len(e.Tickets) == 0 {
		return false
	}
	for _, t := range e.Tickets {
		switch e.RenderedStatus(t) {
		case tickets.StatusDone, tickets.StatusNeedsInfo, tickets.StatusNeedsAttention:
		default:
			return false
		}
	}
	return true
}

// iterationParams are the per-ticket inputs to runIteration.
type iterationParams struct {
	WorkspaceID string
	RepoDir     string
	// WorktreeDir is the directory this Run call's iteration worktrees live
	// in (git.Repo.LinkedWorktreeDir for RepoDir's repo), resolved once in
	// Run and threaded through so every iteration's path is
	// filepath.Join(WorktreeDir, iterLabel(ticketNumber)) — deterministic,
	// so a reattached iteration can recompute it without asking herdr.
	WorktreeDir     string
	FeatureWorktree string
	FeatureBranch   string
	Agent           AgentKind
	Skill           string
	Ticket          tickets.Ticket
	// ScratchDir locates this run's run-log.jsonl (see eventlog.go);
	// logEvent no-ops if it's empty. The epic name half of that path is
	// FeatureBranch, not a separate field — Run names the feature branch
	// after the epic, so the two are always the same value.
	ScratchDir string
	// FeatureLock serializes the only step that mutates the shared feature
	// worktree's working directory (cherry-picking a finished iteration's
	// commits onto it), so concurrently-running iterations never do so at
	// the same time.
	FeatureLock *sync.Mutex

	// SmartZone is the context-token ceiling before an iteration (or its
	// conflict-resolution agent) gets paused.
	SmartZone int
	// Gate is the pause/resume coordinator shared by every iteration in this
	// Run call.
	Gate *pauseGate
	// ResumeSignalPath is where a paused iteration polls for `gx ralph-loop
	// resume`.
	ResumeSignalPath string
	// Report writes a line to the loop's output, safe to call concurrently
	// from any iteration's goroutine.
	Report func(format string, args ...any)
}

// launchAndPromptParams builds the launchAndPrompt call for one pane in this
// iteration (the iteration's own pane, or its conflict-resolution pane),
// carrying over the smart-zone guardrail fields every pane in an iteration
// shares. startEvent/finishEvent name the run-log event types logged when
// the agent starts/finishes (see eventlog.go); pass "" for either to skip
// logging that transition (the conflict-resolution pane logs conflict-hit/
// conflict-resolved itself instead, around the generic start/finish here).
func (p iterationParams) launchAndPromptParams(label, pane, tab, prompt, sessionCwd, startEvent, finishEvent string) launchAndPromptParams {
	return launchAndPromptParams{
		Label:            label,
		Agent:            p.Agent,
		Pane:             pane,
		Tab:              tab,
		Prompt:           prompt,
		SessionCwd:       sessionCwd,
		SmartZone:        p.SmartZone,
		Gate:             p.Gate,
		ResumeSignalPath: p.ResumeSignalPath,
		Report:           p.Report,
		Ticket:           p.Ticket.Identifier,
		TicketPath:       p.Ticket.Path,
		ScratchDir:       p.ScratchDir,
		EpicName:         p.FeatureBranch,
		StartEvent:       startEvent,
		FinishEvent:      finishEvent,
	}
}

// logTicketEvent appends eventType to p's run-log with p's ticket number
// plus the given pane/tab/session/cwd — the shared shape behind every event
// this package logs outside launchAndPrompt's own generic start/finish pair
// (needs-info, cherry-picked, conflict-hit, conflict-resolved,
// deps-installed).
func (p iterationParams) logTicketEvent(eventType, pane, tab, agentSession, cwd string) {
	p.logTicketEventReason(eventType, pane, tab, agentSession, cwd, "")
}

// logTicketEventReason is logTicketEvent plus a Reason, for events that
// carry one (currently only deps-installed, whose Reason is the install
// command run).
func (p iterationParams) logTicketEventReason(eventType, pane, tab, agentSession, cwd, reason string) {
	p.logTicketEventSHA(eventType, pane, tab, agentSession, cwd, reason, "")
}

// logTicketEventSHA is logTicketEventReason plus a SHA, for the
// cherry-picked event — the feature branch's landed tip, recorded so startup
// reconciliation can later confirm it's still reachable (see Event.SHA).
func (p iterationParams) logTicketEventSHA(eventType, pane, tab, agentSession, cwd, reason, sha string) {
	_ = logEvent(p.ScratchDir, p.FeatureBranch, Event{
		Type:         eventType,
		Ticket:       p.Ticket.Identifier,
		Agent:        p.Agent,
		Pane:         pane,
		Tab:          tab,
		AgentSession: agentSession,
		SHA:          sha,
		Cwd:          cwd,
		Reason:       reason,
	})
}

// runIteration drives one ticket through the full iteration lifecycle:
// create its worktree, launch and prompt the agent, wait for it to finish,
// cherry-pick its commits onto the feature branch, mark the ticket done, and
// remove the iteration worktree. If the agent finishes without landing any
// commits, the ticket is marked needs-info instead and the worktree/tab are
// left in place for inspection.
func runIteration(d Deps, p iterationParams) error {
	label := iterLabel(p.Ticket.Identifier)
	branch := iterBranch(p.Ticket.Identifier)
	path := filepath.Join(p.WorktreeDir, label)

	base, err := d.RevParse(p.FeatureWorktree, p.FeatureBranch)
	if err != nil {
		return fmt.Errorf("resolving %s tip: %w", p.FeatureBranch, err)
	}

	if err := d.AddWorktree(p.RepoDir, path, branch, base); err != nil {
		return fmt.Errorf("creating iteration worktree: %w", err)
	}

	// Runs synchronously here, before the agent's tab/session exist, so an
	// install failure surfaces as an iteration-lifecycle error immediately
	// rather than eating into the agent's own turn or smart-zone budget.
	command, err := d.InstallDeps(path)
	if err != nil {
		return fmt.Errorf("installing dependencies in %s: %w", path, err)
	}
	p.logTicketEventReason(eventDepsInstalled, "", "", "", path, command)

	tab, err := d.TabCreate(herdr.TabCreateOptions{
		WorkspaceID: p.WorkspaceID,
		Cwd:         path,
		Label:       label,
	})
	if err != nil {
		return fmt.Errorf("opening iteration tab: %w", err)
	}

	prompt := skillPrompt(p.Agent, p.Skill, p.Ticket.Path)
	launchParams := p.launchAndPromptParams(label, tab.RootPaneID, tab.TabID, prompt, path, eventIterationStarted, eventIterationFinished)
	sessionID, err := launchAndPrompt(d, launchParams)
	if err != nil {
		return err
	}

	return finishIteration(d, p, path, tab.RootPaneID, tab.TabID, base, branch, sessionID)
}

// reattachIteration resumes a ticket left `Status: claimed` by a prior
// crashed/killed invocation whose iter-NN worktree/tab is still alive: it
// recomputes the worktree's deterministic path and finds its still-live tab
// (rather than creating either), skips straight to re-entering the "wait for
// the agent to finish" step (no launch or initial prompt — the agent may
// already be mid-turn or already done), then continues through the same
// cherry-pick/mark-done/remove completion path as a fresh iteration. The
// tab's agent is targeted by name (the iteration label, which AgentStart
// registered it under on the prior invocation) since no fresh AgentStart ran
// here to hand back a pane id. The original base commit is recovered via
// merge-base against the feature branch rather than the feature branch's
// current tip, since the feature branch may have advanced past this
// iteration's original branch point while the prior invocation was down.
func reattachIteration(d Deps, p iterationParams) error {
	label := iterLabel(p.Ticket.Identifier)
	branch := iterBranch(p.Ticket.Identifier)
	path := filepath.Join(p.WorktreeDir, label)

	tabs, err := d.TabList(p.WorkspaceID)
	if err != nil {
		return fmt.Errorf("finding live tab for reattached iteration %s: %w", label, err)
	}
	var tab herdr.Tab
	for _, t := range tabs {
		if t.Label == label {
			tab = t
			break
		}
	}
	if tab.TabID == "" {
		return fmt.Errorf("no live tab found for reattached iteration %s", label)
	}
	tabID := tab.TabID

	base, err := d.MergeBase(path, branch, p.FeatureBranch)
	if err != nil {
		return fmt.Errorf("resolving %s's original base: %w", branch, err)
	}

	// No AgentStart was made this invocation, so there's no fresh
	// agent_session id to check smart-zone occupancy against, or to attach to
	// this ticket's later needs-info/cherry-picked events; the reattached
	// wait simply re-observes idle/done without that guardrail. StartEvent is
	// left empty for the same reason (no fresh launch happened here to log).
	launchParams := p.launchAndPromptParams(label, label, tabID, "", path, "", eventIterationFinished)
	if alreadyFinished(tab.AgentStatus) {
		// tab's status came from TabList just above, taken at reattach time —
		// the agent may have finished while the previous invocation was down,
		// with no further status transition ever coming, so waitForFinish's
		// AgentWait poll would have nothing new to observe.
		p.Report("%s already finished at reattach; skipping wait\n", label)
		launchParams.logLifecycleEvent(launchParams.FinishEvent, "")
	} else if err := waitForFinish(d, launchParams, ""); err != nil {
		return fmt.Errorf("waiting for reattached agent %s to finish: %w", label, err)
	}
	if strings.EqualFold(strings.TrimSpace(p.Ticket.Status), "needs-attention") {
		if err := Claim(p.Ticket.Path); err != nil {
			return fmt.Errorf("restoring ticket to claimed: %w", err)
		}
		p.Gate.resumeLabel(label)
		p.Report("resumed %s after restart recheck\n", label)
		launchParams.logLifecycleEvent(eventResumed, "")
	}

	return finishIteration(d, p, path, label, tabID, base, branch, "")
}

// finishIteration lands a finished iteration's commits (or marks it
// needs-info if it produced none), then removes its worktree/tab on success
// — the shared tail of both the fresh (runIteration) and reattached
// (reattachIteration) iteration lifecycles. sessionID is the iteration
// agent's Claude Code session (empty for a reattached iteration, which made
// no fresh AgentStart call this invocation), attached to the needs-info/
// cherry-picked events logged here. pane is either the iteration's real pane
// id (fresh) or its agent name (reattached) — either is a valid herdr agent
// target.
func finishIteration(d Deps, p iterationParams, path, pane, tab, base, branch, sessionID string) error {
	ahead, err := d.CommitsAhead(path, base, branch)
	if err != nil {
		return fmt.Errorf("counting commits ahead of %s: %w", base, err)
	}
	if ahead == 0 {
		// waitForFinish's own debounce (confirmFinished) already guards against
		// herdr reporting the agent idle mid-turn, but a commit can still land
		// in the gap between that confirmation and this check (e.g. a reattached
		// iteration, which skips waitForFinish's launch-time debounce entirely).
		// Recheck once more before giving up rather than orphaning a commit
		// that lands moments later.
		d.Sleep(finishDebounceMs * time.Millisecond)
		ahead, err = d.CommitsAhead(path, base, branch)
		if err != nil {
			return fmt.Errorf("counting commits ahead of %s: %w", base, err)
		}
	}
	if ahead == 0 {
		// The agent finished without landing any commits: leave the worktree/
		// tab in place for inspection instead of silently marking done or
		// retrying, and let the scheduler move on to other unblocked tickets.
		if err := MarkNeedsInfo(p.Ticket.Path); err != nil {
			return fmt.Errorf("marking ticket needs-info: %w", err)
		}
		p.logTicketEvent(eventNeedsInfo, pane, tab, sessionID, path)
		return nil
	}

	landedSHA, err := landCherryPick(d, p, base, branch, sessionID, pane, tab)
	if err != nil {
		return err
	}
	p.logTicketEventSHA(eventCherryPicked, pane, tab, sessionID, path, "", landedSHA)

	if err := markDoneStampingCloseMetadata(d, p, path, sessionID); err != nil {
		return fmt.Errorf("marking ticket done: %w", err)
	}

	return finishCleanup(d, p.RepoDir, p.FeatureWorktree, path, branch, tab)
}

// markDoneStampingCloseMetadata marks p.Ticket done, stamping the closing
// iteration's context-window occupancy and session id alongside Status when
// sessionID is a live, fresh-iteration session (runIteration's case) and its
// occupancy is available — a reattached close (sessionID == "") has no live
// session of its own to read occupancy from, so it backfills from the
// ticket's original iteration-started session in the run log instead.
func markDoneStampingCloseMetadata(d Deps, p iterationParams, cwd, sessionID string) error {
	if sessionID == "" {
		return backfillDoneMetadata(d, p)
	}
	occupancy, ok, occErr := contextOccupancy(d, p.Agent, cwd, sessionID)
	if occErr != nil || !ok {
		return MarkDone(p.Ticket.Path)
	}
	return MarkDoneWithMetadata(p.Ticket.Path, occupancy, sessionID)
}

// backfillDoneMetadata handles a reattached ticket close: it looks up the
// ticket's original iteration-started session from the run log (ticket 06a)
// and stamps context-window/session from that session's own transcript,
// rather than leaving the fields blank or attributing them to a fresh
// session that didn't do the work. Falls back to a plain MarkDone whenever
// no prior session can be found, or its occupancy can't be read — these
// fields are a best-effort convenience, not required for a ticket to close.
func backfillDoneMetadata(d Deps, p iterationParams) error {
	events, ok, err := readEvents(p.ScratchDir, p.FeatureBranch)
	if err != nil || !ok {
		return MarkDone(p.Ticket.Path)
	}
	sessionID, sessionCwd, agent, ok := lastIterationSession(events, p.Ticket.Identifier)
	if !ok {
		return MarkDone(p.Ticket.Path)
	}
	occupancy, ok, occErr := contextOccupancy(d, agent, sessionCwd, sessionID)
	if occErr != nil || !ok {
		return MarkDone(p.Ticket.Path)
	}
	return MarkDoneWithMetadata(p.Ticket.Path, occupancy, sessionID)
}

// landCherryPick cherry-picks base..branch onto the feature branch (resolving
// conflicts via cherryPickWithConflictResolution if any arise), then resolves
// the SHA it landed at — the shared core of both a normal iteration's
// completion (finishIteration) and a startup repair's re-cherry-pick
// (repairRecoverableTicket). p.FeatureLock is held for the duration since
// this mutates the shared feature worktree's working directory; the landed
// SHA is resolved before releasing it, before another iteration's cherry-pick
// can advance the tip past this one's — otherwise the recorded SHA could
// belong to a different ticket entirely.
func landCherryPick(d Deps, p iterationParams, base, branch, sessionID, pane, tab string) (string, error) {
	p.FeatureLock.Lock()
	defer p.FeatureLock.Unlock()

	if err := cherryPickWithConflictResolution(d, p, base, branch, sessionID, pane, tab); err != nil {
		return "", err
	}
	landedSHA, err := d.RevParse(p.FeatureWorktree, "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolving landed commit on %s: %w", p.FeatureBranch, err)
	}
	return landedSHA, nil
}

// finishCleanup removes an iteration's now-redundant worktree, tab, and
// branch. Each check is independent since callers run this against state left
// in different shapes: a normal completion (worktree/tab/branch all just
// created and definitely present), or a done ticket's leftover state found on
// startup after a crash (any subset of the three may have survived — see
// classifyDoneTicket's doneStaleCleanup). tabID is "" if no live tab was
// found for this iteration.
func finishCleanup(d Deps, repoDir, featureWorktree, path, branch, tabID string) error {
	hasWorktree, err := d.WorktreeExists(path)
	if err != nil {
		return fmt.Errorf("checking iteration worktree: %w", err)
	}
	if hasWorktree {
		if err := d.RemoveWorktree(repoDir, path, true); err != nil {
			return fmt.Errorf("removing iteration worktree: %w", err)
		}
	}

	if tabID != "" {
		if err := d.TabClose(tabID); err != nil {
			return fmt.Errorf("closing iteration tab: %w", err)
		}
	}

	if branchExists(d, featureWorktree, branch) {
		if err := d.DeleteBranch(repoDir, branch); err != nil {
			return fmt.Errorf("deleting iteration branch %s: %w", branch, err)
		}
	}

	return nil
}

// branchExists reports whether branch is a resolvable ref in dir's repo,
// treating any RevParse error as "doesn't exist" — the same convention
// classifyDoneTicket uses to check the iteration branch's presence.
func branchExists(d Deps, dir, branch string) bool {
	_, err := d.RevParse(dir, branch)
	return err == nil
}

// conflictResolutionTimeoutMs bounds how long a conflict-resolution agent may
// run before it's treated as stuck, so a hung resolution surfaces as a
// distinct, actionable error instead of hanging the whole loop forever.
const conflictResolutionTimeoutMs = 30 * 60 * 1000

// cherryPickWithConflictResolution cherry-picks base..branch onto
// p.FeatureWorktree. On a conflict, it launches a fresh pane in the feature
// worktree (where the conflict markers are, not the iteration worktree),
// sends "/resolving-merge-conflicts", and waits for that agent to finish
// before confirming the cherry-pick sequence completed. sessionID/pane/tab
// attach the iteration agent's own session to the conflict-hit event (the
// resolution agent hasn't launched yet at that point); conflict-resolved is
// instead attributed to the resolution agent's own distinct session, so its
// cost/occupancy are counted too rather than silently dropped from the
// report.
func cherryPickWithConflictResolution(d Deps, p iterationParams, base, branch, sessionID, pane, tab string) error {
	pickErr := d.CherryPickRange(p.FeatureWorktree, base, branch)
	if pickErr == nil {
		return nil
	}

	inProgress, err := d.CherryPickInProgress(p.FeatureWorktree)
	if err != nil {
		return fmt.Errorf("checking cherry-pick state onto %s: %w", p.FeatureBranch, err)
	}
	if !inProgress {
		return fmt.Errorf("cherry-picking onto %s: %w", p.FeatureBranch, pickErr)
	}
	p.logTicketEvent(eventConflictHit, pane, tab, sessionID, p.FeatureWorktree)

	resolutionSessionID, err := resolveCherryPickConflict(d, p)
	if err != nil {
		return err
	}

	inProgress, err = d.CherryPickInProgress(p.FeatureWorktree)
	if err != nil {
		return fmt.Errorf("checking cherry-pick state onto %s after resolution: %w", p.FeatureBranch, err)
	}
	if inProgress {
		return fmt.Errorf("cherry-pick onto %s still in progress after conflict-resolution agent finished", p.FeatureBranch)
	}
	p.logTicketEvent(eventConflictResolved, "", "", resolutionSessionID, p.FeatureWorktree)
	return nil
}

// resolveCherryPickConflict launches a fresh pane in the feature worktree and
// drives a "/resolving-merge-conflicts" agent to completion in it, returning
// its agent session id. The iteration's own worktree/tab are untouched
// while this runs.
func resolveCherryPickConflict(d Deps, p iterationParams) (string, error) {
	label := conflictLabel(p.Ticket.Identifier)

	tab, err := d.TabCreate(herdr.TabCreateOptions{
		WorkspaceID: p.WorkspaceID,
		Cwd:         p.FeatureWorktree,
		Label:       label,
	})
	if err != nil {
		return "", fmt.Errorf("creating conflict-resolution pane: %w", err)
	}

	launchParams := p.launchAndPromptParams(label, tab.RootPaneID, tab.TabID, skillPrompt(p.Agent, "resolving-merge-conflicts", ""), p.FeatureWorktree, "", "")
	launchParams.FinishTimeoutMs = conflictResolutionTimeoutMs
	sessionID, err := launchAndPrompt(d, launchParams)
	if err != nil {
		return "", fmt.Errorf("conflict-resolution agent %s did not finish (possibly stuck): %w", label, err)
	}

	return sessionID, nil
}

// launchAndPromptParams are the per-call inputs to launchAndPrompt.
type launchAndPromptParams struct {
	Label  string // agent name/tab label, used in error messages
	Agent  AgentKind
	Pane   string // pane id to launch the agent in and send the prompt to
	Tab    string // tab id owning Pane, recorded on logged events
	Prompt string // initial skill prompt text

	// FinishTimeoutMs bounds the final "wait for the agent to finish" step, so
	// a stuck agent surfaces as a distinct error instead of blocking forever.
	// Zero means wait indefinitely.
	FinishTimeoutMs int

	// SessionCwd is the cwd Pane's agent was launched in.
	SessionCwd string
	// SmartZone is the context-token ceiling before this agent gets paused.
	SmartZone int
	// Gate is the pause/resume coordinator shared across the whole Run call.
	Gate *pauseGate
	// ResumeSignalPath is where a paused agent polls for `gx ralph-loop
	// resume`.
	ResumeSignalPath string
	// Report writes a line to the loop's output, safe to call concurrently.
	Report func(format string, args ...any)

	// Ticket, TicketPath, ScratchDir, and EpicName locate the run-log.jsonl entries this
	// call logs (see eventlog.go). Ticket is the ticket's Identifier (not
	// Number), so lettered split siblings sharing a Number are distinguishable
	// in the run log.
	Ticket     string
	TicketPath string
	ScratchDir string
	EpicName   string
	// StartEvent/FinishEvent are the run-log event types logged when the
	// agent starts/finishes; "" skips logging that transition.
	StartEvent  string
	FinishEvent string
}

// logLifecycleEvent appends eventType to p's run-log with p's Ticket/Pane/
// Tab, plus agentSession if known. A no-op if eventType is "".
func (p launchAndPromptParams) logLifecycleEvent(eventType, agentSession string) {
	if eventType == "" {
		return
	}
	p.logAgentEvent(eventType, agentSession, "")
}

func (p launchAndPromptParams) logAgentEvent(eventType, agentSession, reason string) {
	_ = logEvent(p.ScratchDir, p.EpicName, Event{
		Type:         eventType,
		Ticket:       p.Ticket,
		Agent:        p.Agent,
		Pane:         p.Pane,
		Tab:          p.Tab,
		AgentSession: agentSession,
		Cwd:          p.SessionCwd,
		Reason:       reason,
	})
}

func (p launchAndPromptParams) report(format string, args ...any) {
	if p.Report == nil {
		return
	}
	p.Report(format, args...)
}

// launchAndPrompt runs the shared agent lifecycle protocol: launch the agent in
// Pane, wait for it to reach idle, send Prompt and wait for it to start
// working, then wait for it to finish (idle or done) — pausing the whole
// loop via Gate if this agent's context occupancy breaches SmartZone before
// it finishes.
func launchAndPrompt(d Deps, p launchAndPromptParams) (string, error) {
	agent, err := d.AgentStart(herdr.AgentStartOptions{
		Name:      p.Label,
		Kind:      string(p.Agent),
		Pane:      p.Pane,
		AgentArgs: agentArgs(p.Agent, p.ScratchDir, p.EpicName),
	})
	if err != nil {
		return "", fmt.Errorf("launching %s: %w", p.Agent, err)
	}
	p.logLifecycleEvent(p.StartEvent, agent.AgentSession)

	if _, err := d.AgentWait(herdr.AgentWaitOptions{
		Target: p.Pane,
		Until:  []string{"idle"},
	}); err != nil {
		return "", fmt.Errorf("waiting for %s to reach idle after launch: %w", p.Agent, err)
	}

	if _, err := d.AgentPrompt(herdr.AgentPromptOptions{
		Target: p.Pane,
		Text:   p.Prompt,
		Wait:   true,
		Until:  []string{"working"},
	}); err != nil {
		return "", fmt.Errorf("sending initial prompt: %w", err)
	}

	if err := waitForFinish(d, p, agent.AgentSession); err != nil {
		return "", err
	}
	return agent.AgentSession, nil
}

// plainFinishStates are the herdr agent_status values that mean "the agent's
// turn is over" for every agent kind. waitForFinish appends "blocked" to
// these for Codex, which needs its own rate-limit/attention-recovery handling
// rather than being treated as finished.
var plainFinishStates = []string{"idle", "done"}

// alreadyFinished reports whether status (a herdr tab's current agent_status,
// e.g. from TabList) already matches one of waitForFinish's plain-completion
// target states. "blocked" is deliberately excluded even though waitForFinish
// treats it as a Codex finish state too — a pane already sitting blocked at
// reattach still needs waitForFinish's rate-limit/attention-recovery
// handling, not a bare skip.
func alreadyFinished(status string) bool {
	return slices.Contains(plainFinishStates, status)
}

// waitForFinish polls Pane until it reaches idle or done, checking the
// session's current context occupancy against SmartZone on every poll tick
// that times out rather than settling. A breach interrupts the pane (Ctrl-C,
// not killed), pauses the whole loop via Gate, and blocks here until a `gx
// ralph-loop resume` signal arrives — at which point it re-enters this same
// wait rather than assuming the pause fixed anything.
func waitForFinish(d Deps, p launchAndPromptParams, sessionID string) error {
	smartZone := p.SmartZone
	if smartZone <= 0 {
		smartZone = defaultSmartZone
	}

	elapsedMs := 0
	for {
		pollMs := smartZonePollMs
		if p.FinishTimeoutMs > 0 {
			remaining := p.FinishTimeoutMs - elapsedMs
			if remaining <= 0 {
				return fmt.Errorf("waiting for agent to finish: timed out after %dms", p.FinishTimeoutMs)
			}
			if remaining < pollMs {
				pollMs = remaining
			}
		}

		until := append([]string{}, plainFinishStates...)
		if p.Agent == AgentCodex {
			until = append(until, "blocked")
		}
		agent, err := d.AgentWait(herdr.AgentWaitOptions{
			Target:    p.Pane,
			Until:     until,
			TimeoutMs: pollMs,
		})
		if err == nil {
			if p.Agent == AgentCodex && agent.AgentStatus == "blocked" {
				if limit, exhausted, limitErr := codexRateLimit(d, p.SessionCwd, sessionID); limitErr == nil && exhausted {
					if err := recoverCodexRateLimit(d, p, sessionID, limit); err != nil {
						return err
					}
					elapsedMs = 0
					continue
				}
				if err := waitForAttentionRecovery(d, p, sessionID); err != nil {
					return err
				}
				elapsedMs = 0
				continue
			}
			confirmed, err := confirmFinished(d, p.Pane, until)
			if err != nil {
				return fmt.Errorf("confirming %s finished: %w", p.Label, err)
			}
			if !confirmed {
				// The agent went back to work in the debounce window (see
				// finishDebounceMs): this was a transient idle blip, not a real
				// finish, so keep waiting instead of declaring victory early.
				elapsedMs = 0
				continue
			}
			p.logLifecycleEvent(p.FinishEvent, sessionID)
			return nil
		}
		if !isPollTimeout(err) {
			return fmt.Errorf("waiting for agent to finish: %w", err)
		}
		elapsedMs += pollMs

		if p.Agent == AgentCodex {
			if limit, exhausted, limitErr := codexRateLimit(d, p.SessionCwd, sessionID); limitErr == nil && exhausted {
				if err := recoverCodexRateLimit(d, p, sessionID, limit); err != nil {
					return err
				}
				elapsedMs = 0
				continue
			}
		}

		if p.Agent == AgentClaude && d.ReadPaneRecent != nil {
			if text, rlErr := d.ReadPaneRecent(p.Pane); rlErr == nil {
				if token, matched := detectRateLimit(text); matched {
					reason := "rate limit detected"
					if token != "" {
						reason = fmt.Sprintf("rate limit detected, resets %s", token)
					}
					p.Gate.pause(p.Label, reason)
					p.report("paused %s: %s; waiting for automatic reset\n", p.Label, reason)
					p.logAgentEvent(eventPausedRateLimit, sessionID, reason)
					waitForRateLimitReset(d, p.Pane, token)
					p.Gate.resumeLabel(p.Label)
					p.report("resumed %s after rate-limit reset\n", p.Label)
					p.logLifecycleEvent(eventResumed, sessionID)

					if _, err := d.AgentPrompt(herdr.AgentPromptOptions{
						Target: p.Pane,
						Text:   "continue",
						Wait:   true,
						Until:  []string{"working"},
					}); err != nil {
						return fmt.Errorf("re-prompting %s after rate-limit reset: %w", p.Label, err)
					}
					elapsedMs = 0
					continue
				}
			}
		}

		occupancy, ok, occErr := contextOccupancy(d, p.Agent, p.SessionCwd, sessionID)
		if occErr != nil || !ok || occupancy <= smartZone {
			continue
		}

		if err := d.AgentSendKeys(p.Pane, "ctrl+c"); err != nil {
			return fmt.Errorf("interrupting %s after smart-zone breach: %w", p.Label, err)
		}
		reason := fmt.Sprintf("context occupancy %d exceeds --smart-zone %d", occupancy, smartZone)
		p.Gate.pause(p.Label, reason)
		p.report("paused %s: %s; run `gx ralph-loop resume` to continue\n", p.Label, reason)
		p.logAgentEvent(eventPausedSmartZone, sessionID, reason)
		p.Gate.waitForResume(d, p.ResumeSignalPath)
		p.report("resumed %s\n", p.Label)
		p.logLifecycleEvent(eventResumed, sessionID)
		elapsedMs = 0
	}
}

// confirmFinished debounces a just-reached idle/done signal on pane: it
// pauses finishDebounceMs, then re-polls for up to finishConfirmMs to see
// whether the agent is still in one of until's finish states. A poll timeout
// (the agent went back to "working" in the meantime) means the original
// signal was a transient blip, not a real finish.
func confirmFinished(d Deps, pane string, until []string) (bool, error) {
	d.Sleep(finishDebounceMs * time.Millisecond)
	_, err := d.AgentWait(herdr.AgentWaitOptions{
		Target:    pane,
		Until:     until,
		TimeoutMs: finishConfirmMs,
	})
	if err == nil {
		return true, nil
	}
	if isPollTimeout(err) {
		return false, nil
	}
	return false, err
}

func recoverCodexRateLimit(d Deps, p launchAndPromptParams, sessionID string, limit codexsession.RateLimit) error {
	reason := fmt.Sprintf("Codex %s quota exhausted", limit.Quota)
	if !limit.ResetAt.IsZero() {
		reason += fmt.Sprintf(", resets %s", limit.ResetAt.UTC().Format(time.RFC3339))
	}
	p.Gate.pause(p.Label, reason)
	p.report("paused %s: %s; waiting for automatic reset\n", p.Label, reason)
	p.logAgentEvent(eventPausedRateLimit, sessionID, reason)
	waitForCodexRateLimitReset(d, p.SessionCwd, sessionID, limit)
	p.Gate.resumeLabel(p.Label)
	p.report("resumed %s after Codex quota reset\n", p.Label)
	p.logLifecycleEvent(eventResumed, sessionID)

	agent, err := d.AgentWait(herdr.AgentWaitOptions{
		Target: p.Pane,
		Until:  []string{"idle", "done", "working", "blocked"},
	})
	if err != nil {
		return fmt.Errorf("re-observing %s after Codex quota reset: %w", p.Label, err)
	}
	if agent.AgentStatus != "blocked" {
		return nil
	}

	if _, err := d.AgentPrompt(herdr.AgentPromptOptions{
		Target: p.Pane,
		Text:   "continue",
		Wait:   true,
		Until:  []string{"working"},
	}); err != nil {
		return fmt.Errorf("re-prompting %s after Codex quota reset: %w", p.Label, err)
	}
	return nil
}

// waitForAttentionRecovery makes a Codex permission/intervention request
// durable while keeping its pane and worktree available. The scheduler stays
// paused until the pane returns to idle/done; a resume signal merely asks for
// an immediate recheck, so it cannot accidentally schedule work while Codex
// remains blocked.
func waitForAttentionRecovery(d Deps, p launchAndPromptParams, sessionID string) error {
	const reason = "Codex is waiting for operator intervention"
	if err := MarkNeedsAttention(p.TicketPath); err != nil {
		return fmt.Errorf("marking ticket needs-attention: %w", err)
	}
	p.Gate.pause(p.Label, reason)
	p.report("paused %s: %s\n", p.Label, reason)
	p.logAgentEvent(eventNeedsAttention, sessionID, reason)

	for {
		agent, err := d.AgentWait(herdr.AgentWaitOptions{
			Target:    p.Pane,
			Until:     []string{"idle", "done"},
			TimeoutMs: smartZonePollMs,
		})
		if err == nil && agent.AgentStatus != "blocked" {
			if err := Claim(p.TicketPath); err != nil {
				return fmt.Errorf("restoring ticket to claimed: %w", err)
			}
			p.Gate.resumeLabel(p.Label)
			p.report("resumed %s after operator intervention\n", p.Label)
			p.logLifecycleEvent(eventResumed, sessionID)
			return nil
		}
		if err != nil && !isPollTimeout(err) {
			return fmt.Errorf("rechecking blocked agent: %w", err)
		}

		signaled, signalErr := d.ResumeSignaled(p.ResumeSignalPath)
		if signalErr != nil || !signaled {
			continue
		}
		agent, err = d.AgentWait(herdr.AgentWaitOptions{
			Target: p.Pane,
			Until:  []string{"idle", "done", "blocked"},
		})
		if err != nil {
			return fmt.Errorf("manually rechecking blocked agent: %w", err)
		}
		if agent.AgentStatus == "blocked" {
			p.report("%s still needs attention\n", p.Label)
			continue
		}
		if err := Claim(p.TicketPath); err != nil {
			return fmt.Errorf("restoring ticket to claimed: %w", err)
		}
		p.Gate.resumeLabel(p.Label)
		p.report("resumed %s after manual recheck\n", p.Label)
		p.logLifecycleEvent(eventResumed, sessionID)
		return nil
	}
}

// contextOccupancy reads the selected agent's own local session data. A
// missing observer is treated like incomplete session data, keeping the
// running iteration alive instead of falsely pausing it.
func contextOccupancy(d Deps, agent AgentKind, cwd, sessionID string) (int, bool, error) {
	if sessionID == "" {
		return 0, false, nil
	}
	if agent == AgentCodex {
		if d.ReadCodexContext == nil {
			return 0, false, nil
		}
		return d.ReadCodexContext(cwd, sessionID)
	}
	if d.ReadOccupancy == nil {
		return 0, false, nil
	}
	return d.ReadOccupancy(cwd, sessionID)
}

func codexRateLimit(d Deps, cwd, sessionID string) (codexsession.RateLimit, bool, error) {
	if sessionID == "" || d.ReadCodexRateLimit == nil {
		return codexsession.RateLimit{}, false, nil
	}
	return d.ReadCodexRateLimit(cwd, sessionID)
}

// isPollTimeout reports whether err looks like AgentWait's own
// timeout-elapsed failure (herdr's "timed out waiting for agent status"),
// as opposed to a genuine failure that should abort the loop instead of
// looping back for another poll tick.
func isPollTimeout(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "timed out")
}

// iterLabel/iterBranch/conflictLabel key off a ticket's Identifier (the
// filename's full "NN[letter]" prefix), not its Number, so that lettered
// split siblings sharing the same Number (e.g. "04a"/"04b") get distinct
// labels/branches/worktree paths instead of colliding on "iter-04".
func iterLabel(identifier string) string {
	return "iter-" + identifier
}

func iterBranch(identifier string) string {
	return "ralph-loop/" + iterLabel(identifier)
}

func conflictLabel(identifier string) string {
	return "conflict-" + identifier
}

// loadNamedEpic loads scratchDir and returns the epic named name, or nil if
// no such epic exists.
func loadNamedEpic(scratchDir, name string) (*tickets.Epic, error) {
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		return nil, err
	}
	for i := range epics {
		if epics[i].Name == name {
			return &epics[i], nil
		}
	}
	return nil, nil
}
