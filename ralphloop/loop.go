package ralphloop

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// defaultScratchDir is the ticket tracker directory used when
// RunOptions.ScratchDir is unset.
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
// RunOptions.SmartZone is unset. It backstops the gx-implement skill's own
// proactive 90K split threshold, so a session that misses that threshold
// still gets paused before it runs away.
const defaultSmartZone = 130_000

// defaultWorkerSkill is the skill each iteration invokes when
// RunOptions.Skill is unset.
const defaultWorkerSkill = "gx-implement"

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
	Skill       string    // skill each iteration invokes; defaults to defaultWorkerSkill ("gx-implement") when unset
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
	// OnScopeResolved, if set, is called synchronously with the RunScope Run
	// resolves from TicketIDs, before the ticket loop starts — the caller's
	// way to keep a reference for later out-of-band widening (RunScope.Add),
	// e.g. a TUI's "add to queue" action on an already-live run. Called under
	// no lock of Run's own, so the callback is responsible for whatever
	// synchronization it needs against its own state.
	OnScopeResolved func(RunScope)
	// Permit, if set, gates how many epics may hold an active concurrency
	// slot at once — nil means no cap, today's unrestricted behavior.
	Permit Permit
}

// Permit gates how many epics may hold an active concurrency slot at once —
// distinct from RunOptions.MaxParallel, which bounds iterations within a
// single epic. Acquire blocks until a slot is free; Release frees one this
// same Run acquired. A nil Permit (RunOptions.Permit unset) means no cap.
type Permit interface {
	Acquire()
	Release()
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
	runStart := d.Now()

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
	skill := opts.Skill
	if skill == "" {
		skill = defaultWorkerSkill
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
	if opts.OnScopeResolved != nil {
		opts.OnScopeResolved(scope)
	}
	total := scope.TotalCount(*initial)
	if scope.AllDone(*initial) {
		if allDone(*initial) {
			if err := tickets.StampEpicCompleted(scratchDir, opts.EpicName, d.Now()); err != nil {
				return fmt.Errorf("stamping epic %q completed_at: %w", opts.EpicName, err)
			}
		}
		sink.AlreadyComplete(opts.EpicName, scope.DoneCount(*initial), total)
		return nil
	}
	if agent == AgentCodex && d.PreflightAgent != nil {
		if err := d.PreflightAgent(agent); err != nil {
			return fmt.Errorf("preflighting %s launch environment: %w", agent, err)
		}
	}
	if d.VerifySkill != nil {
		if err := d.VerifySkill(agent, skill); err != nil {
			return fmt.Errorf("preflighting %s skill %q: %w", agent, skill, err)
		}
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
	// pick into it at the same time. worktreeMu guards every `git worktree
	// add`/`git worktree remove` against RepoDir, since git's own worktree
	// administrative files aren't safe under concurrent add/remove on the
	// same repo. sink is safe for concurrent use on its own (see EventSink),
	// since a paused iteration reports its pause/resume from its own
	// goroutine.
	var scheduleMu sync.Mutex
	var featureMu sync.Mutex
	var worktreeMu sync.Mutex

	// launched tracks every ticket identifier this Run call currently has
	// launched (fresh or reattached); an iteration that ends in a stalled
	// status is removed again, so clearing that status by hand puts the
	// ticket back in play for this same run. claimNext consults it
	// instead of trusting the ticket file's on-disk status alone: a still-
	// running iteration's ticket file is a shared, unlocked plain file, and
	// an unrelated writer (e.g. a parent split ticket's agent touching its
	// child's file after handoff) can revert its status back to "open"
	// mid-iteration. Without this guard, the next scheduler scan would see
	// that revert and reclaim + relaunch the same ticket under its
	// deterministic herdr agent name, colliding with the still-alive
	// original (agent_name_taken). Every read and write happens on Run's own
	// goroutine (the initial reattach loop below, claimNext, and the
	// scheduling loop's own result handling), so it needs no locking of its
	// own.
	launched := make(map[string]bool)

	// everLaunched records every ticket identifier this Run call has ever
	// handed to launch(), and — unlike launched — is never cleared. It's how
	// claimNext tells a genuinely fresh frontier ticket apart from one that
	// cleared out of a stalled status: only the latter might still have a
	// live iteration worth reattaching to (see resumeReattachable), since a
	// ticket this run never launched can't have one.
	everLaunched := make(map[string]bool)

	gate := opts.Gate
	if gate == nil {
		gate = NewGate()
	}

	// permitHeld/acquirePermit/releasePermit track this Run's concurrency
	// slot against opts.Permit: acquired before any claim while active==0,
	// released by the time Run returns. The defer is what makes that safe
	// under every return path (deadlock error, mid-loop error,
	// StampEpicCompleted failure, normal completion) without duplicating a
	// release call at each one.
	permitHeld := false
	acquirePermit := func() {
		if opts.Permit != nil && !permitHeld {
			opts.Permit.Acquire()
			permitHeld = true
		}
	}
	releasePermit := func() {
		if opts.Permit != nil && permitHeld {
			opts.Permit.Release()
			permitHeld = false
		}
	}
	defer releasePermit()

	type outcome struct {
		ticket tickets.Ticket
		err    error
	}
	results := make(chan outcome)

	// claimNext claims and returns the next frontier ticket, or ok=false if
	// none is available right now (every remaining ticket is blocked,
	// already claimed by a running iteration, the loop is paused on a
	// smart-zone breach, or the epic is done).
	claimNext := func() (ticket tickets.Ticket, reattach bool, ok bool, err error) {
		scheduleMu.Lock()
		defer scheduleMu.Unlock()

		var scanned *tickets.Epic
		var frontier []tickets.Ticket
		admitted, err := gate.claimIfRunning(func() error {
			epic, err := loadNamedEpic(scratchDir, opts.EpicName)
			if err != nil {
				return err
			}
			scanned = epic
			frontier = scope.Frontier(*epic)
			for _, candidate := range frontier {
				if !launched[candidate.Identifier] {
					ticket = candidate
					break
				}
			}
			if ticket.Path == "" {
				return nil
			}
			// A ticket this run previously launched and later dropped out of
			// launched (a stalled outcome cleared by a person, see ticket 08)
			// might still have a live iteration nobody tore down — its "open"
			// status alone can't tell that apart from a ticket that never ran.
			// Iteration ownership decides, not the status.
			if everLaunched[ticket.Identifier] {
				reattach = resumeReattachable(d, workspaceID, opts.EpicName, agent, wtDir, ticket)
			}
			if err := Claim(ticket.Path); err != nil {
				return fmt.Errorf("claiming ticket %s: %w", ticket.Identifier, err)
			}
			launched[ticket.Identifier] = true
			everLaunched[ticket.Identifier] = true
			// StampEpicStarted is idempotent (see its doc comment), so calling
			// it on every claim — not just the epic's very first — is safe;
			// it only ever writes started_at once.
			if err := tickets.StampEpicStarted(scratchDir, opts.EpicName, d.Now()); err != nil {
				return fmt.Errorf("stamping epic %q started_at: %w", opts.EpicName, err)
			}
			return nil
		})
		// scanned is nil when the gate wasn't running (claimIfRunning never
		// invoked the closure, e.g. paused) — nothing was actually scanned,
		// so there's nothing useful to log.
		if scanned != nil {
			logErr := logEvent(scratchDir, opts.EpicName, Event{
				Type:  eventSchedulerScan,
				Agent: agent,
				Scan:  scanDecisions(*scanned, scope, frontier, ticket),
			})
			if logErr != nil && err == nil {
				err = fmt.Errorf("logging scheduler scan: %w", logErr)
			}
		}
		if err != nil {
			return tickets.Ticket{}, false, false, err
		}
		if !admitted || ticket.Path == "" {
			return tickets.Ticket{}, false, false, nil
		}
		sink.TicketClaimed(ticket)
		return ticket, reattach, true, nil
	}

	launch := func(ticket tickets.Ticket, reattach bool) {
		go func() {
			params := iterationParams{
				WorkspaceID:     workspaceID,
				RepoDir:         opts.RepoDir,
				WorktreeDir:     wtDir,
				FeatureWorktree: featurePath,
				FeatureBranch:   opts.EpicName,
				Agent:           agent,
				Skill:           skill,
				Ticket:          ticket,
				ScratchDir:      scratchDir,
				FeatureLock:     &featureMu,
				WorktreeLock:    &worktreeMu,
				SmartZone:       smartZone,
				Gate:            gate,
				Sink:            sink,
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
		WorkspaceID:  workspaceID,
		Paths:        reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: featurePath, WorktreeDir: wtDir, RepoDir: opts.RepoDir},
		Agent:        agent,
		Skill:        skill,
		SmartZone:    smartZone,
		Gate:         gate,
		FeatureLock:  &featureMu,
		WorktreeLock: &worktreeMu,
		Sink:         sink,
		Scope:        scope,
	}, *initial)
	if err != nil {
		return err
	}
	// A ticket found needs-repair at startup deliberately does not pause
	// the gate: it's human-clearable, so the run schedules every other
	// runnable ticket first and only parks on it once nothing is runnable.
	if len(reattached) > 0 {
		acquirePermit()
	}
	for _, ticket := range reattached {
		launch(ticket, true)
		active++
		launched[ticket.Identifier] = true
		everLaunched[ticket.Identifier] = true
	}

	// parked records that the run has already announced this park, so the
	// poll loop below notifies once per park rather than once per tick. It
	// resets the moment anything is running again, so a run that resumes and
	// later parks on a different ticket announces that park too.
	parked := false

	for {
		epic, err := loadNamedEpic(scratchDir, opts.EpicName)
		if err != nil {
			return err
		}
		if scope.AllDone(*epic) && active == 0 {
			if allDone(*epic) {
				if err := tickets.StampEpicCompleted(scratchDir, opts.EpicName, d.Now()); err != nil {
					return fmt.Errorf("stamping epic %q completed_at: %w", opts.EpicName, err)
				}
			}
			break
		}

		if active == 0 {
			acquirePermit()
		}
		for active < maxParallel {
			ticket, reattach, ok, err := claimNext()
			if err != nil {
				return err
			}
			if !ok {
				break
			}
			launch(ticket, reattach)
			active++
		}

		if active == 0 {
			releasePermit()
			if gate.isPaused() {
				// Nothing is left running to lift the pause from the inside,
				// so block until an in-process ForceResume (e.g. the TUI)
				// clears it rather than returning: an exiting run takes its
				// panes' recoverability with it.
				gate.waitForResume()
				continue
			}
			stalled := stalledTickets(*epic, scope)
			if len(stalled) == 0 {
				return fmt.Errorf("epic %q is deadlocked: no runnable tickets left, none done, and none a human could clear; check for a dependency cycle or a bad status", opts.EpicName)
			}
			if !parked {
				stalledForSink := make([]StalledTicket, len(stalled))
				for i, t := range stalled {
					stalledForSink[i] = StalledTicket{
						Identifier:   t.Identifier,
						Reattachable: everLaunched[t.Identifier] && resumeReattachable(d, workspaceID, opts.EpicName, agent, wtDir, t),
					}
				}
				sink.EpicParked(opts.EpicName, stalledForSink)
				parked = true
			}
			// No timeout: the notification has already fired, and the park
			// ends when a human clears one of the stalled tickets, which the
			// next pass picks up off disk. gate.WakeParked() lets an operator
			// cut the wait short cosmetically.
			select {
			case <-d.ParkTimer(parkPollInterval):
			case <-gate.ParkWake():
			}
			continue
		}
		parked = false

		r := <-results
		active--
		if r.err != nil {
			// A single iteration erroring out (a herdr/git hiccup, an
			// unexpected agent-wait failure, ...) shouldn't take down the
			// whole epic's run — that leaves every other in-flight ticket's
			// live-event stream dead too, with no way to recover short of
			// restarting the loop. Flag just this ticket needs-repair
			// (out of the frontier, so never reclaimed until a human clears
			// it) and keep scheduling the rest.
			reason := r.err.Error()
			label := iterLabel(opts.EpicName, r.ticket.Identifier)
			state := schema.NeedsRepairState{
				Label:    label,
				Branch:   iterBranch(opts.EpicName, r.ticket.Identifier),
				Worktree: iterationWorktreePath(wtDir, opts.EpicName, r.ticket.Identifier),
			}
			if markErr := MarkNeedsRepairWithReason(r.ticket.Path, reason, state); markErr != nil {
				reason = fmt.Sprintf("%s (also failed marking needs-repair: %v)", reason, markErr)
			}
			delete(launched, r.ticket.Identifier)
			sink.IterationPaused(label, PauseNeedsRepair, reason)
			continue
		}

		// landCherryPick has already written elapsed/context metrics into the
		// ticket's own frontmatter by the time results yields this outcome
		// (see writeLandedMetrics), so re-parsing off disk here is simpler
		// than threading a return value up through runIteration/finishIteration.
		landedTicket := r.ticket
		liveTotal := total
		if landedEpic, err := loadNamedEpic(scratchDir, opts.EpicName); err != nil {
			return err
		} else if landedEpic != nil {
			liveTotal = scope.TotalCount(*landedEpic)
			for _, t := range landedEpic.Tickets {
				if t.Identifier == r.ticket.Identifier {
					landedTicket = t
					// An iteration that ended stalled (needs-answer, most
					// commonly) is no longer launched, so a human clearing
					// its status puts it back in the frontier for this run
					// instead of only for the next one.
					if isParked(*landedEpic, t) {
						delete(launched, t.Identifier)
					}
					break
				}
			}
		}

		completed++
		sink.IterationFinished(landedTicket, opts.EpicName, IterationStats{
			ElapsedSeconds:    landedTicket.ElapsedTime,
			PeakContextTokens: landedTicket.ActualContextWindow,
			InProgress:        active,
			Completed:         completed,
			Total:             liveTotal,
		})
	}

	sink.EpicComplete(opts.EpicName, completed, int(d.Now().Sub(runStart).Seconds()))
	return nil
}

// scanDecisions builds one claimNext pass's scheduler-scan log line: epic's
// tickets, in file order, each labeled with why the scheduler did or didn't
// claim it this pass. claimed is the zero-value Ticket when this pass found
// nothing to claim.
func scanDecisions(epic tickets.Epic, scope RunScope, frontier []tickets.Ticket, claimed tickets.Ticket) []ScanDecision {
	inFrontier := make(map[string]bool, len(frontier))
	for _, t := range frontier {
		inFrontier[t.Path] = true
	}

	decisions := make([]ScanDecision, 0, len(epic.Tickets))
	for _, t := range epic.Tickets {
		status := epic.RenderedStatus(t)
		d := ScanDecision{Ticket: t.Identifier, Status: status.Word()}
		switch {
		case claimed.Path != "" && t.Path == claimed.Path:
			d.Decision = "claimed"
		case !scope.Contains(t, epic):
			d.Decision = "out-of-scope"
		case status == tickets.StatusDone:
			d.Decision = "done"
		case isParked(epic, t):
			d.Decision = "stalled"
		case inFrontier[t.Path]:
			d.Decision = "frontier"
		case status == tickets.StatusBlocked:
			d.Decision = "blocked"
			d.Reason = strings.Join(epic.UnresolvedBlockers(t), ", ")
		default:
			d.Decision = "unclaimed"
		}
		decisions = append(decisions, d)
	}
	return decisions
}

// allDone reports whether every ticket in e is done — the run's one exit
// condition. A stalled ticket (see isParked) is deliberately not an
// exit condition: the run parks on it instead, so the agent's question is
// still answerable in its own pane.
func allDone(e tickets.Epic) bool {
	if len(e.Tickets) == 0 {
		return false
	}
	for _, t := range e.Tickets {
		if e.RenderedStatus(t) != tickets.StatusDone {
			return false
		}
	}
	return true
}

// isParked reports whether t is stalled on a person rather than on
// the run: needs-answer (an iteration finished with no commits to land),
// needs-repair (operator intervention, or a done ticket reconciliation
// found unrecoverable — see markDoneTicketUnrecoverable), or draft (a stub
// nobody has filled in yet). None of these ever appear in the frontier, so a
// run with only these left has nothing to schedule — but a person can clear
// any of them, which is what separates parking from a deadlock.
func isParked(e tickets.Epic, t tickets.Ticket) bool {
	switch e.RenderedStatus(t) {
	case tickets.StatusNeedsAnswer, tickets.StatusNeedsRepair, tickets.StatusDraft:
		return true
	}
	return false
}

// stalledTickets lists every in-scope, human-clearable ticket in e, in file
// order — what a park notification names as the thing waiting on a person.
func stalledTickets(e tickets.Epic, scope RunScope) []tickets.Ticket {
	var stalled []tickets.Ticket
	for _, t := range e.Tickets {
		if scope.Contains(t, e) && isParked(e, t) {
			stalled = append(stalled, t)
		}
	}
	return stalled
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
