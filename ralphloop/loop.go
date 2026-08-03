package ralphloop

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/elentok/gx/tickets"
)

// defaultScratchDir is the ticket tracker directory used when
// RunOptions.ScratchDir (or Resume's scratchDir) is unset.
const defaultScratchDir = ".scratch"

// ticketTrailerKey is the commit-message trailer landCherryPick stamps onto
// every landed commit (see Deps.AppendTrailer) and classifyDoneTicket's last-
// resort fallback searches for (Deps.TrailerCommitExists).
const ticketTrailerKey = "Ralph-Loop-Ticket"

// tokensTrailerKey and elapsedTrailerKey are the commit-message trailers
// landCherryPick stamps alongside ticketTrailerKey whenever the landing
// session's own metrics are available (see writeLandedMetrics) — the same
// actual_context_window/elapsed_time values written to the ticket's
// frontmatter, surfaced on the landed commit too.
const (
	tokensTrailerKey  = "Ralph-Loop-Tokens"
	elapsedTrailerKey = "Ralph-Loop-Elapsed"
)

// ticketTrailerValue builds ticketTrailerKey's value, scoped to epicName: a
// bare ticket identifier repeats across every epic (each restarts numbering
// from 01), and TrailerCommitExists searches the whole repo history, not just
// commits under this epic. An unscoped value can match a same-numbered
// ticket from a completely unrelated epic landed long ago, misreporting a
// genuinely unlanded ticket as already present — classifyDoneTicket then
// deletes its worktree/branch without ever cherry-picking it, discarding
// real work.
func ticketTrailerValue(epicName, identifier string) string {
	return epicName + "/" + identifier
}

// defaultMaxParallel is how many iterations run concurrently when
// RunOptions.MaxParallel is unset.
const defaultMaxParallel = 2

// defaultSmartZone is the context-token ceiling used when
// RunOptions.SmartZone is unset.
const defaultSmartZone = 110_000

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
	// TicketIDs, if set, restricts scheduling to just these ticket
	// identifiers (see tickets.Ticket.DisplayNumber) within the epic — Run
	// exits once every one of them is done, independent of any other open
	// ticket the epic may still have. Unset means the whole epic, unchanged
	// from before this field existed.
	TicketIDs []string
	// Gate, if set, is used instead of a fresh one Run would otherwise create
	// internally — the caller's way to keep a reference for in-process
	// resume/recheck (Gate.ForceResume) once the loop is running, e.g. a TUI
	// sharing this process with Run. Headless callers leave this nil and get
	// today's behavior unchanged.
	Gate *Gate
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
func Run(opts RunOptions, d Deps, sink EventSink) error {
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
		sink.NoTicketsFound(opts.EpicName)
		return nil
	}
	scope, err := ResolveRunScope(*initial, opts.TicketIDs)
	if err != nil {
		return err
	}
	if scope.AllSettled(*initial) {
		sink.AlreadyComplete(opts.EpicName, scope.DoneCount(*initial), scope.TotalCount(*initial))
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
	// pick into it at the same time. sink is safe for concurrent use on its
	// own (see EventSink), since a paused iteration reports its pause/resume
	// from its own goroutine.
	var scheduleMu sync.Mutex
	var featureMu sync.Mutex

	gate := opts.Gate
	if gate == nil {
		gate = NewGate()
	}
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

		admitted, err := gate.claimIfRunning(func() error {
			epic, err := loadNamedEpic(scratchDir, opts.EpicName)
			if err != nil {
				return err
			}
			frontier := scope.Frontier(*epic)
			if len(frontier) == 0 {
				return nil
			}
			ticket = frontier[0]
			if err := Claim(ticket.Path); err != nil {
				return fmt.Errorf("claiming ticket %s: %w", ticket.Identifier, err)
			}
			return nil
		})
		if err != nil {
			return tickets.Ticket{}, false, err
		}
		if !admitted || ticket.Path == "" {
			return tickets.Ticket{}, false, nil
		}
		sink.TicketClaimed(ticket)
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
				Sink:             sink,
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
		Sink:             sink,
		Scope:            scope,
	}, *initial)
	if err != nil {
		return err
	}
	for _, ticket := range initial.Tickets {
		if scope.Contains(ticket) && strings.EqualFold(strings.TrimSpace(ticket.Status), "needs-attention") {
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
		if scope.AllSettled(*epic) && active == 0 {
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
			if gate.isLabelPaused(QueuePauseLabel) {
				gate.waitForResume(d, resumePath)
				continue
			}
			if gate.isPaused() {
				return fmt.Errorf("epic %q paused with no running iterations left: %v", opts.EpicName, gate.snapshot())
			}
			return fmt.Errorf("epic %q has no unblocked tickets left but isn't all done; check for a stuck ticket", opts.EpicName)
		}

		r := <-results
		active--
		if r.err != nil {
			// A single iteration erroring out (a herdr/git hiccup, an
			// unexpected agent-wait failure, ...) shouldn't take down the
			// whole epic's run — that leaves every other in-flight ticket's
			// live-event stream dead too, with no way to recover short of
			// restarting the loop. Flag just this ticket needs-attention
			// (terminal per allSettled, excluded from future claims) and
			// keep scheduling the rest.
			reason := r.err.Error()
			label := iterLabel(r.ticket.Identifier)
			if markErr := MarkNeedsAttentionWithReason(r.ticket.Path, reason); markErr != nil {
				reason = fmt.Sprintf("%s (also failed marking needs-attention: %v)", reason, markErr)
			}
			sink.IterationPaused(label, PauseNeedsAttention, reason)
			continue
		}

		sink.IterationFinished(r.ticket, opts.EpicName)
		completed++
	}

	sink.EpicComplete(opts.EpicName, completed)
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
		if !isSettledStatus(e.RenderedStatus(t)) {
			return false
		}
	}
	return true
}

// isSettledStatus reports whether a rendered status counts as terminal from
// the loop's perspective — shared by allSettled and RunScope.AllSettled so
// the done/needs-info/needs-attention set is defined in exactly one place.
func isSettledStatus(status tickets.RenderedStatus) bool {
	switch status {
	case tickets.StatusDone, tickets.StatusNeedsInfo, tickets.StatusNeedsAttention:
		return true
	default:
		return false
	}
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
