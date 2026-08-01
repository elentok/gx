package ralphloop

import (
	"fmt"
	"io"
	"strings"
	"sync"

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

// RunOptions configures a single `gx ralph-loop {epic-name}` invocation.
type RunOptions struct {
	EpicName    string
	Skill       string // slash-command skill each iteration invokes, e.g. "implement"
	ScratchDir  string // defaults to ".scratch"
	RepoDir     string // repo root passed as the herdr workspace/worktree cwd
	MaxParallel int    // defaults to defaultMaxParallel; how many iterations run concurrently
	SmartZone   int    // defaults to defaultSmartZone; context-token ceiling before pausing an iteration
}

// Run drives every unblocked ticket in the named epic to completion, up to
// MaxParallel running concurrently, each in its own iteration worktree:
// create the iteration worktree, launch claude, send the initial
// "/{skill} <ticket-path>" prompt, wait for it to finish, cherry-pick its
// commits onto the feature branch, mark the ticket done, and remove the
// iteration worktree. As soon as one iteration finishes, a freed slot is
// backfilled with the next frontier ticket. It exits once every ticket in
// the epic reaches a done-family status, or immediately if the epic has none
// to run.
func Run(opts RunOptions, d Deps, out io.Writer) error {
	scratchDir := opts.ScratchDir
	if scratchDir == "" {
		scratchDir = defaultScratchDir
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

	featureWT, err := d.WorktreeCreate(herdr.WorktreeCreateOptions{
		WorkspaceID: workspaceID,
		Cwd:         opts.RepoDir,
		Branch:      opts.EpicName,
		Label:       opts.EpicName,
		Focus:       true,
	})
	if err != nil {
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
			return tickets.Ticket{}, false, fmt.Errorf("claiming ticket %d: %w", ticket.Number, err)
		}
		return ticket, true, nil
	}

	launch := func(ticket tickets.Ticket, reattach bool) {
		go func() {
			params := iterationParams{
				WorkspaceID:      workspaceID,
				RepoDir:          opts.RepoDir,
				FeatureWorktree:  featureWT.Path,
				FeatureBranch:    opts.EpicName,
				Skill:            opts.Skill,
				Ticket:           ticket,
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

	reattached, err := reconcile(d, workspaceID, *initial, report)
	if err != nil {
		return err
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
			return fmt.Errorf("ticket %02d: %w", r.ticket.Number, r.err)
		}

		report("ticket %02d %q landed on %s\n", r.ticket.Number, r.ticket.Title, opts.EpicName)
		completed++
	}

	report("ralph-loop %q complete: %d ticket(s) landed on %s\n", opts.EpicName, completed, opts.EpicName)
	return nil
}

// allSettled reports whether every ticket in e has reached a terminal state
// from the loop's perspective: done, or needs-info (an iteration that
// finished with no commits to land, left for inspection). Unlike
// tickets.Epic.AllDone — which only tickets in the done family and is shared
// with the tickets UI's collapse/expand rendering — needs-info counts as
// terminal here too, so the loop can exit cleanly once every remaining
// ticket is either landed or stuck needing a human, rather than looping
// forever (Frontier already excludes needs-info from scheduling).
func allSettled(e tickets.Epic) bool {
	if len(e.Tickets) == 0 {
		return false
	}
	for _, t := range e.Tickets {
		switch e.RenderedStatus(t) {
		case tickets.StatusDone, tickets.StatusNeedsInfo:
		default:
			return false
		}
	}
	return true
}

// iterationParams are the per-ticket inputs to runIteration.
type iterationParams struct {
	WorkspaceID     string
	RepoDir         string
	FeatureWorktree string
	FeatureBranch   string
	Skill           string
	Ticket          tickets.Ticket
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
// shares.
func (p iterationParams) launchAndPromptParams(label, pane, prompt, sessionCwd string) launchAndPromptParams {
	return launchAndPromptParams{
		Label:            label,
		Pane:             pane,
		Prompt:           prompt,
		SessionCwd:       sessionCwd,
		SmartZone:        p.SmartZone,
		Gate:             p.Gate,
		ResumeSignalPath: p.ResumeSignalPath,
		Report:           p.Report,
	}
}

// runIteration drives one ticket through the full iteration lifecycle:
// create its worktree, launch and prompt the agent, wait for it to finish,
// cherry-pick its commits onto the feature branch, mark the ticket done, and
// remove the iteration worktree. If the agent finishes without landing any
// commits, the ticket is marked needs-info instead and the worktree/tab are
// left in place for inspection.
func runIteration(d Deps, p iterationParams) error {
	label := iterLabel(p.Ticket.Number)
	branch := iterBranch(p.Ticket.Number)

	base, err := d.RevParse(p.FeatureWorktree, p.FeatureBranch)
	if err != nil {
		return fmt.Errorf("resolving %s tip: %w", p.FeatureBranch, err)
	}

	iterWT, err := d.WorktreeCreate(herdr.WorktreeCreateOptions{
		WorkspaceID: p.WorkspaceID,
		Cwd:         p.RepoDir,
		Branch:      branch,
		Base:        p.FeatureBranch,
		Label:       label,
	})
	if err != nil {
		return fmt.Errorf("creating iteration worktree: %w", err)
	}

	prompt := fmt.Sprintf("/%s %s", p.Skill, p.Ticket.Path)
	launchParams := p.launchAndPromptParams(label, iterWT.PaneID, prompt, iterWT.Path)
	if err := launchAndPrompt(d, launchParams); err != nil {
		return err
	}

	return finishIteration(d, p, iterWT, base, branch)
}

// reattachIteration resumes a ticket left `Status: claimed` by a prior
// crashed/killed invocation whose iter-NN worktree/tab is still alive: it
// reopens the existing worktree/tab (rather than creating a new one), skips
// straight to re-entering the "wait for the agent to finish" step (no launch
// or initial prompt — the agent may already be mid-turn or already done),
// then continues through the same cherry-pick/mark-done/remove completion
// path as a fresh iteration. The original base commit is recovered via
// merge-base against the feature branch rather than the feature branch's
// current tip, since the feature branch may have advanced past this
// iteration's original branch point while the prior invocation was down.
func reattachIteration(d Deps, p iterationParams) error {
	label := iterLabel(p.Ticket.Number)
	branch := iterBranch(p.Ticket.Number)

	iterWT, err := d.WorktreeOpen(herdr.WorktreeOpenOptions{
		WorkspaceID: p.WorkspaceID,
		Cwd:         p.RepoDir,
		Branch:      branch,
		Label:       label,
	})
	if err != nil {
		return fmt.Errorf("reopening iteration worktree: %w", err)
	}

	base, err := d.MergeBase(iterWT.Path, branch, p.FeatureBranch)
	if err != nil {
		return fmt.Errorf("resolving %s's original base: %w", branch, err)
	}

	// No AgentStart was made this invocation, so there's no fresh
	// agent_session id to check smart-zone occupancy against; the reattached
	// wait simply re-observes idle/done without that guardrail.
	launchParams := p.launchAndPromptParams(label, iterWT.PaneID, "", iterWT.Path)
	if err := waitForFinish(d, launchParams, ""); err != nil {
		return fmt.Errorf("waiting for reattached agent %s to finish: %w", label, err)
	}

	return finishIteration(d, p, iterWT, base, branch)
}

// finishIteration lands a finished iteration's commits (or marks it
// needs-info if it produced none), then removes its worktree on success —
// the shared tail of both the fresh (runIteration) and reattached
// (reattachIteration) iteration lifecycles.
func finishIteration(d Deps, p iterationParams, iterWT herdr.Worktree, base, branch string) error {
	ahead, err := d.CommitsAhead(iterWT.Path, base, branch)
	if err != nil {
		return fmt.Errorf("counting commits ahead of %s: %w", base, err)
	}
	if ahead == 0 {
		// The agent finished without landing any commits: leave the worktree/
		// tab in place for inspection instead of silently marking done or
		// retrying, and let the scheduler move on to other unblocked tickets.
		if err := MarkNeedsInfo(p.Ticket.Path); err != nil {
			return fmt.Errorf("marking ticket needs-info: %w", err)
		}
		return nil
	}

	p.FeatureLock.Lock()
	err = cherryPickWithConflictResolution(d, p, base, branch)
	p.FeatureLock.Unlock()
	if err != nil {
		return err
	}

	if err := MarkDone(p.Ticket.Path); err != nil {
		return fmt.Errorf("marking ticket done: %w", err)
	}

	if err := d.WorktreeRemove(iterWT.WorkspaceID, true); err != nil {
		return fmt.Errorf("removing iteration worktree: %w", err)
	}

	return nil
}

// conflictResolutionTimeoutMs bounds how long a conflict-resolution agent may
// run before it's treated as stuck, so a hung resolution surfaces as a
// distinct, actionable error instead of hanging the whole loop forever.
const conflictResolutionTimeoutMs = 30 * 60 * 1000

// cherryPickWithConflictResolution cherry-picks base..branch onto
// p.FeatureWorktree. On a conflict, it launches a fresh pane in the feature
// worktree (where the conflict markers are, not the iteration worktree),
// sends "/resolving-merge-conflicts", and waits for that agent to finish
// before confirming the cherry-pick sequence completed.
func cherryPickWithConflictResolution(d Deps, p iterationParams, base, branch string) error {
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

	if err := resolveCherryPickConflict(d, p); err != nil {
		return err
	}

	inProgress, err = d.CherryPickInProgress(p.FeatureWorktree)
	if err != nil {
		return fmt.Errorf("checking cherry-pick state onto %s after resolution: %w", p.FeatureBranch, err)
	}
	if inProgress {
		return fmt.Errorf("cherry-pick onto %s still in progress after conflict-resolution agent finished", p.FeatureBranch)
	}
	return nil
}

// resolveCherryPickConflict launches a fresh pane in the feature worktree and
// drives a "/resolving-merge-conflicts" agent to completion in it. The
// iteration's own worktree/tab are untouched while this runs.
func resolveCherryPickConflict(d Deps, p iterationParams) error {
	label := conflictLabel(p.Ticket.Number)

	tab, err := d.TabCreate(herdr.TabCreateOptions{
		WorkspaceID: p.WorkspaceID,
		Cwd:         p.FeatureWorktree,
		Label:       label,
	})
	if err != nil {
		return fmt.Errorf("creating conflict-resolution pane: %w", err)
	}

	launchParams := p.launchAndPromptParams(label, tab.RootPaneID, "/resolving-merge-conflicts", p.FeatureWorktree)
	launchParams.FinishTimeoutMs = conflictResolutionTimeoutMs
	if err := launchAndPrompt(d, launchParams); err != nil {
		return fmt.Errorf("conflict-resolution agent %s did not finish (possibly stuck): %w", label, err)
	}

	return nil
}

// launchAndPromptParams are the per-call inputs to launchAndPrompt.
type launchAndPromptParams struct {
	Label  string // agent name/tab label, used in error messages
	Pane   string // pane id to launch claude in and send the prompt to
	Prompt string // initial slash-command prompt text

	// FinishTimeoutMs bounds the final "wait for the agent to finish" step, so
	// a stuck agent surfaces as a distinct error instead of blocking forever.
	// Zero means wait indefinitely.
	FinishTimeoutMs int

	// SessionCwd is the cwd Pane's claude was launched in, i.e. where its
	// Claude Code transcript is filed under ~/.claude/projects/<slug>/.
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
}

func (p launchAndPromptParams) report(format string, args ...any) {
	if p.Report == nil {
		return
	}
	p.Report(format, args...)
}

// launchAndPrompt runs the shared agent lifecycle protocol: launch claude in
// Pane, wait for it to reach idle, send Prompt and wait for it to start
// working, then wait for it to finish (idle or done) — pausing the whole
// loop via Gate if this agent's context occupancy breaches SmartZone before
// it finishes.
func launchAndPrompt(d Deps, p launchAndPromptParams) error {
	agent, err := d.AgentStart(herdr.AgentStartOptions{
		Name: p.Label,
		Kind: "claude",
		Pane: p.Pane,
	})
	if err != nil {
		return fmt.Errorf("launching claude: %w", err)
	}

	if _, err := d.AgentWait(herdr.AgentWaitOptions{
		Target: p.Pane,
		Until:  []string{"idle"},
	}); err != nil {
		return fmt.Errorf("waiting for claude to reach idle after launch: %w", err)
	}

	if _, err := d.AgentPrompt(herdr.AgentPromptOptions{
		Target: p.Pane,
		Text:   p.Prompt,
		Wait:   true,
		Until:  []string{"working"},
	}); err != nil {
		return fmt.Errorf("sending initial prompt: %w", err)
	}

	return waitForFinish(d, p, agent.AgentSession)
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

		_, err := d.AgentWait(herdr.AgentWaitOptions{
			Target:    p.Pane,
			Until:     []string{"idle", "done"},
			TimeoutMs: pollMs,
		})
		if err == nil {
			return nil
		}
		if !isPollTimeout(err) {
			return fmt.Errorf("waiting for agent to finish: %w", err)
		}
		elapsedMs += pollMs

		if d.ReadPaneRecent != nil {
			if text, rlErr := d.ReadPaneRecent(p.Pane); rlErr == nil {
				if token, matched := detectRateLimit(text); matched {
					reason := "rate limit detected"
					if token != "" {
						reason = fmt.Sprintf("rate limit detected, resets %s", token)
					}
					p.Gate.pause(p.Label, reason)
					p.report("paused %s: %s; waiting for automatic reset\n", p.Label, reason)
					waitForRateLimitReset(d, p.Pane, token)
					p.Gate.resumeLabel(p.Label)
					p.report("resumed %s after rate-limit reset\n", p.Label)

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

		if sessionID == "" {
			continue // no session id to check a transcript against yet
		}
		occupancy, ok, occErr := d.ReadOccupancy(p.SessionCwd, sessionID)
		if occErr != nil || !ok || occupancy <= smartZone {
			continue
		}

		if err := d.AgentSendKeys(p.Pane, "ctrl-c"); err != nil {
			return fmt.Errorf("interrupting %s after smart-zone breach: %w", p.Label, err)
		}
		reason := fmt.Sprintf("context occupancy %d exceeds --smart-zone %d", occupancy, smartZone)
		p.Gate.pause(p.Label, reason)
		p.report("paused %s: %s; run `gx ralph-loop resume` to continue\n", p.Label, reason)
		p.Gate.waitForResume(d, p.ResumeSignalPath)
		p.report("resumed %s\n", p.Label)
		elapsedMs = 0
	}
}

// isPollTimeout reports whether err looks like AgentWait's own
// timeout-elapsed failure (herdr's "timed out waiting for agent status"),
// as opposed to a genuine failure that should abort the loop instead of
// looping back for another poll tick.
func isPollTimeout(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "timed out")
}

func iterLabel(ticketNumber int) string {
	return fmt.Sprintf("iter-%02d", ticketNumber)
}

func iterBranch(ticketNumber int) string {
	return "ralph-loop/" + iterLabel(ticketNumber)
}

func conflictLabel(ticketNumber int) string {
	return fmt.Sprintf("conflict-%02d", ticketNumber)
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
