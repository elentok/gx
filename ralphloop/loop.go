package ralphloop

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/elentok/gx/config"
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
	costTrailerKey    = "Ralph-Loop-Cost"
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

// codeReviewSkill is the skill launched directly for type: code-review
// tickets instead of defaultWorkerSkill/RunOptions.Skill (see
// runIteration). It's launched as its own fresh /command rather than
// entered from inside a gx-implement session because gx-code-review sets
// disable-model-invocation: true, which blocks an in-session Skill-tool
// hand-off but not the harness starting a session with the command
// already resolved.
const codeReviewSkill = "gx-code-review"

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

func agentArgs(agent AgentKind, scratchDir, epicName, model, effort string) []string {
	if agent == AgentCodex {
		args := []string{
			"--sandbox", "workspace-write",
			"--ask-for-approval", "on-request",
			"--add-dir", filepath.Join(scratchDir, epicName),
		}
		if model != "" {
			args = append(args, "--model", model)
		}
		if effort != "" {
			// Codex has no dedicated effort flag; -c is the only route it
			// offers, and gx deliberately does not manage a codex profile
			// file to get one.
			args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", effort))
		}
		return args
	}
	args := []string{"--permission-mode", "auto"}
	if model != "" {
		args = append(args, "--model", model)
	}
	if effort != "" {
		args = append(args, "--effort", effort)
	}
	return args
}

// resolvedAgentConfig picks agents' Claude or Codex sub-config for agent —
// the single AgentConfig relevant to this Run call, since RunOptions.Agent is
// fixed for the whole run.
func resolvedAgentConfig(agents config.AgentsConfig, agent AgentKind) config.AgentConfig {
	if agent == AgentCodex {
		return agents.Codex
	}
	return agents.Claude
}

// RunOptions configures a single `gx ralph-loop {epic-name}` invocation.
type RunOptions struct {
	EpicName string
	Agent    AgentKind // defaults to AgentClaude
	// Agents carries the resolved per-agent model/effort config an iteration
	// launches under (see resolvedAgentConfig) — the caller supplies it
	// (e.g. from config.Load), ralphloop never loads config itself. The zero
	// value (both fields empty for both agents) reproduces today's launch
	// argv unchanged.
	Agents      config.AgentsConfig
	Skill       string // skill each iteration invokes; defaults to defaultWorkerSkill ("gx-implement") when unset
	ScratchDir  string // defaults to ".scratch"
	RepoDir     string // repo root passed as the herdr workspace/worktree cwd
	MaxParallel int    // defaults to defaultMaxParallel; how many iterations run concurrently
	SmartZone   int    // defaults to defaultSmartZone; context-token ceiling before pausing an iteration
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
	// Ctx, if set, lets a caller request Run stop early: once canceled, Run
	// stops claiming new tickets and returns as soon as every
	// already-launched iteration and the land-queue worker have actually
	// finished — so no goroutine survives the call, without the caller
	// having to know Run's internals to wait on. Unset means
	// context.Background(), i.e. Run only stops when the epic is done or
	// deadlocked, unchanged from before this field existed. A canceled Ctx
	// makes Run return ctx.Err() rather than nil.
	Ctx context.Context
}

// Permit gates how many epics may hold an active concurrency slot at once —
// distinct from RunOptions.MaxParallel, which bounds iterations within a
// single epic. Acquire blocks until a slot is free; Release frees one this
// same Run acquired. A nil Permit (RunOptions.Permit unset) means no cap.
type Permit interface {
	Acquire()
	Release()
}

// outcome is one iteration's or one land's result, sent on results (a build
// goroutine) or landResults (the land-queue worker) respectively.
type outcome struct {
	ticket tickets.Ticket
	err    error
	// built is true when this outcome is a build that finished with commits
	// ready to land (ahead > 0): iteration_status: finished is already
	// written and the job is already queued in landJobs (see launch), so the
	// results-handling loop only needs to release this build's active slot
	// and hand the ticket to the land queue — completed++/
	// sink.IterationFinished wait for the worker's own landResults outcome
	// once the land actually finishes. Never set on a landResults outcome.
	built bool
	// parkedOnChild is true when this outcome is a landResults outcome whose
	// cherry-pick hit errConflictResolutionUnresolved: the ticket was parked
	// needs-repair on a conflict-resolution child, not landed — it stays
	// claimed on disk. err is nil on this path (parking already handled the
	// failure), so without this flag the results loop can't tell it apart
	// from a real completed land. Never set on a results (build) outcome.
	parkedOnChild bool
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
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	agent := opts.Agent
	if agent == "" {
		agent = AgentClaude
	}
	if err := ValidateAgentKind(agent); err != nil {
		return err
	}
	agentConfig := resolvedAgentConfig(opts.Agents, agent)
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

	// permitHeld/acquirePermit/releasePermit track this Run's concurrency
	// slot against opts.Permit: acquired before EpicStarted fires (an epic
	// waiting for a slot hasn't started, so announcing at entry would report
	// a start that hasn't happened and could sit unfulfilled behind a busy
	// queue), released by the time Run returns. The defer is what makes
	// release safe under every return path (deadlock error, mid-loop error,
	// StampEpicCompleted failure, normal completion, or the no-tickets/
	// already-complete early returns below) without duplicating a release
	// call at each one.
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

	initial, err := loadNamedEpic(scratchDir, opts.EpicName)
	if err != nil {
		return err
	}
	if initial == nil || len(initial.Tickets) == 0 {
		acquirePermit()
		sink.EpicStarted(opts.EpicName, 0, 0)
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
		acquirePermit()
		sink.EpicStarted(opts.EpicName, scope.DoneCount(*initial), total)
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
	// ticket, and also guards the land-queue worker's own eligibility scan
	// and landJobs map (see landQueue.go) — the same shape of operation.
	// worktreeMu guards every `git worktree add`/`git worktree remove`
	// against RepoDir, since git's own worktree administrative files aren't
	// safe under concurrent add/remove on the same repo. sink is safe for
	// concurrent use on its own (see EventSink), since a paused iteration
	// reports its pause/resume from its own goroutine. The land-queue
	// worker's single-goroutine-ness is what now serializes cherry-pick
	// landings onto the feature worktree — no lock needed for that anymore.
	var scheduleMu sync.Mutex
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

	results := make(chan outcome)
	landResults := make(chan outcome)

	// landJobs holds the live-process handoff data (see landJob's doc) for
	// every ticket a build goroutine has finished but the land-queue worker
	// hasn't yet landed, keyed by ticket identifier. Written by launch's
	// goroutine before it sends a built:true outcome, read and deleted by the
	// land-queue worker once it starts landing that ticket. landJobsMu guards
	// it since the two run concurrently.
	landJobs := make(map[string]landJob)
	var landJobsMu sync.Mutex

	// landWake nudges the land-queue worker to rescan immediately after a
	// fresh job is queued, rather than only on its own idle poll — a buffered
	// size-1 channel so a wake that arrives while the worker is already
	// mid-pass isn't lost, just coalesced into the next one.
	landWake := make(chan struct{}, 1)
	wakeLandQueue := func() {
		select {
		case landWake <- struct{}{}:
		default:
		}
	}
	landQueueDone := make(chan struct{})
	defer close(landQueueDone)

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
				Model:           agentConfig.Model,
				Effort:          agentConfig.Effort,
				Skill:           skill,
				Ticket:          ticket,
				ScratchDir:      scratchDir,
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
			var built *builtAwaitingLandError
			if errors.As(err, &built) {
				landJobsMu.Lock()
				landJobs[ticket.Identifier] = built.job
				landJobsMu.Unlock()
				wakeLandQueue()
				results <- outcome{ticket: ticket, built: true}
				return
			}
			results <- outcome{ticket: ticket, err: err}
		}()
	}

	completed := 0
	active := 0
	// landing tracks builds that finished (ahead > 0, iteration_status:
	// finished written) but haven't yet landed — queued in landJobs, waiting
	// on the land-queue worker. Incremented when a results outcome arrives
	// with built == true, decremented when the worker reports that ticket's
	// landing over landResults. Both the exit condition and the idle/
	// deadlock branch below gate on active == 0 && landing == 0: a ticket
	// queued behind an unlanded land is neither done nor a stall a human
	// needs to clear.
	landing := 0
	// liveDone tracks the epic-wide done count: seeded from disk once here so
	// a resumed run reports what's true of the epic even though this run's
	// own completed count starts back at zero, then incremented by exactly
	// one per landed outcome this loop drains below (never re-derived from a
	// live disk rescan — two near-simultaneous landings can both already be
	// "done" on disk by the time either outcome is dequeued, which made a
	// rescan double-count instead of reporting 1, then 2).
	liveDone := scope.DoneCount(*initial)

	// The epic is genuinely running past this point (not no-tickets, not
	// already-complete), so the permit is acquired and the single start
	// message fires now — after Acquire() returns, never before — rather
	// than deferring it to whichever later acquirePermit() call happens to
	// run first.
	acquirePermit()
	sink.EpicStarted(opts.EpicName, scope.DoneCount(*initial), total)

	reattached, err := reconcile(d, reconcileParams{
		WorkspaceID:  workspaceID,
		Paths:        reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: featurePath, WorktreeDir: wtDir, RepoDir: opts.RepoDir},
		Agent:        agent,
		Model:        agentConfig.Model,
		Effort:       agentConfig.Effort,
		Skill:        skill,
		SmartZone:    smartZone,
		Gate:         gate,
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
	for _, ticket := range reattached {
		launch(ticket, true)
		active++
		launched[ticket.Identifier] = true
		everLaunched[ticket.Identifier] = true
	}

	// The land-queue worker is the sole serializer of cherry-pick landings
	// for this Run call — replacing featureMu — and runs as a single
	// goroutine decoupled from the build goroutines above, so a build's
	// active slot never waits on a land (including up to 30 minutes of
	// conflict resolution). It requires no persisted state of its own: the
	// in-memory landJobs map (pane/tab/sessionID/path) is what survives the
	// build-goroutine-to-worker handoff, and is also the sole source of
	// landing eligibility (see runLandQueue).
	go runLandQueue(d, landQueueParams{
		WorkspaceID:     workspaceID,
		RepoDir:         opts.RepoDir,
		WorktreeDir:     wtDir,
		FeatureWorktree: featurePath,
		FeatureBranch:   opts.EpicName,
		Agent:           agent,
		Model:           agentConfig.Model,
		Effort:          agentConfig.Effort,
		Skill:           skill,
		ScratchDir:      scratchDir,
		WorktreeLock:    &worktreeMu,
		SmartZone:       smartZone,
		Gate:            gate,
		Sink:            sink,
	}, landJobs, &landJobsMu, landWake, landQueueDone, landResults)

	// parked records that the run has already announced this park, so the
	// poll loop below notifies once per park rather than once per tick. It
	// resets the moment anything is running again, so a run that resumes and
	// later parks on a different ticket announces that park too.
	parked := false
	// drained records that this run ended via the DrainComplete branch below,
	// so the final sink.EpicComplete call after the loop can skip its
	// human-facing chat/toast — DrainComplete already told the operator the
	// run ended, and a trailing "epic complete" is simply false with tickets
	// still open (see the drained guard on that call).
	drained := false

	for {
		epic, err := loadNamedEpic(scratchDir, opts.EpicName)
		if err != nil {
			return err
		}
		// unparkAnswered runs on every pass through this loop — the initial
		// entry, each completed iteration, and (via the select below) every
		// park-poll tick even while siblings are still running — reusing the
		// epic load just above rather than scanning again on its own. See
		// unparkAnswered's doc for why a live, unblocked pane is what makes
		// this safe with no ownership check.
		if err := unparkAnswered(d, workspaceID, opts.EpicName, wtDir, agent, scope, *epic, d.Now()); err != nil {
			return err
		}
		if scope.AllDone(*epic) && active == 0 && landing == 0 {
			if allDone(*epic) {
				if err := tickets.StampEpicCompleted(scratchDir, opts.EpicName, d.Now()); err != nil {
					return fmt.Errorf("stamping epic %q completed_at: %w", opts.EpicName, err)
				}
			}
			break
		}
		// A canceled ctx with nothing left in flight is the shutdown path's
		// own exit: nothing more to drain, so stop here rather than fall
		// through to claiming (guarded below) or parking (which would just
		// block again on the very thing being canceled).
		if ctx.Err() != nil && active == 0 && landing == 0 {
			break
		}

		// A drained gate never admits another claim (see
		// Gate.claimIfRunning), so once its last in-flight iteration
		// finishes there is nothing left this run will ever do — end here,
		// through the same sink.EpicComplete call below that natural
		// completion reaches, rather than falling into the "no unblocked
		// tickets left" active==0 error path further down, which assumes an
		// undrained run is stuck. landing == 0 is required alongside
		// active == 0 (mirroring the ctx.Err() branch above) since a build
		// can finish (active decremented) with its outcome still queued in
		// landJobs, waiting on the land-queue worker's own goroutine —
		// breaking on active == 0 alone would end the run before that last
		// build's commits actually land, leaving it iteration_status:
		// finished but never status: done. Report DrainComplete here,
		// distinct from EpicComplete, since this branch is only reached when
		// draining cut the run short of a naturally-settled epic (see
		// DrainComplete's doc comment) — an operator away from the terminal
		// needs to know the difference.
		if gate.isDraining() && active == 0 && landing == 0 {
			// Check ctx.Err() before emitting DrainComplete: if cancellation
			// landed between the ctx.Err() check above and this one, break
			// without the emit so the ctx.Err() check after the loop reports
			// the caller's own cancellation instead of a drained notification
			// for a run the registry then records as failed.
			if ctx.Err() != nil {
				break
			}
			sink.DrainComplete(opts.EpicName, completed, int(d.Now().Sub(runStart).Seconds()))
			drained = true
			break
		}

		if active == 0 {
			acquirePermit()
		}
		// Once canceled, stop recruiting new work — but every iteration and
		// land already in flight keeps running to completion below, so their
		// goroutines are drained rather than orphaned on a channel nobody
		// reads anymore.
		if ctx.Err() == nil {
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
		}

		if active == 0 && landing == 0 {
			releasePermit()
			if gate.isPaused() {
				// Nothing is left running to lift the pause from the inside,
				// so block until an in-process ForceResume (e.g. the TUI)
				// clears it, or ctx is canceled — an exiting run takes its
				// panes' recoverability with it, but a canceled ctx is the
				// caller explicitly asking to stop anyway.
				gate.waitForResume(ctx)
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
						Reattachable: everLaunched[t.Identifier] && clearableParkedTicket(d, workspaceID, opts.EpicName, wtDir, agent, *epic, t),
					}
				}
				sink.EpicParked(opts.EpicName, stalledForSink)
				parked = true
			}
			// No timeout: the notification has already fired, and the park
			// ends when a human clears one of the stalled tickets, which the
			// next pass picks up off disk. gate.WakeParked() lets an operator
			// cut the wait short cosmetically; ctx.Done() lets a caller stop
			// the run outright.
			select {
			case <-d.ParkTimer(parkPollInterval):
			case <-gate.ParkWake():
			case <-ctx.Done():
			}
			continue
		}
		parked = false

		// With a sibling still running (active > 0, we're past the branch
		// above), a plain receive would block until that sibling finishes even
		// if some other ticket is already sitting needs-answer with its pane
		// answered — the ticket-15 gap. Selecting the same park-timer seam the
		// idle branch above already polls on catches that case: each tick
		// loops back to the top, where the epic reload and unparkAnswered
		// pick it up. Gated on hasLiveParkedTicket rather than "something is
		// parked" at all: a needs-answer ticket with no live pane can only be
		// cleared by a person editing the file (already caught by the next
		// ordinary reload), and polling for it here would race an
		// always-ready park-timer fake against the sibling's results send,
		// starving it out forever.
		// The select below always includes landResults alongside results —
		// harmless even when landing == 0, since nothing sends on it until
		// the worker has a job — so a land finishing never has to wait for a
		// sibling build's own results send.
		var r outcome
		var fromLand bool
		if active > 0 && hasLiveParkedTicket(d, workspaceID, opts.EpicName, wtDir, agent, scope, *epic) {
			select {
			case r = <-results:
			case r = <-landResults:
				fromLand = true
			case <-d.ParkTimer(parkPollInterval):
				continue
			}
		} else {
			select {
			case r = <-results:
			case r = <-landResults:
				fromLand = true
			}
		}
		if fromLand {
			landing--
		} else {
			active--
		}
		if r.err != nil {
			// A single iteration (or land) erroring out (a herdr/git hiccup,
			// an unexpected agent-wait failure, a cherry-pick conflict that
			// couldn't be resolved, ...) shouldn't take down the whole
			// epic's run — that leaves every other in-flight ticket's
			// live-event stream dead too, with no way to recover short of
			// restarting the loop. Flag just this ticket needs-repair
			// (out of the frontier, so never reclaimed until a human clears
			// it) and keep scheduling the rest.
			reason := r.err.Error()
			state := schema.NeedsRepairState{
				Label:    iterLabel(opts.EpicName, r.ticket.Identifier),
				Branch:   iterBranch(opts.EpicName, r.ticket.Identifier),
				Worktree: iterationWorktreePath(wtDir, opts.EpicName, r.ticket.Identifier),
			}
			if markErr := MarkNeedsRepairWithReason(r.ticket.Path, reason, state); markErr != nil {
				reason = fmt.Sprintf("%s (also failed marking needs-repair: %v)", reason, markErr)
			}
			delete(launched, r.ticket.Identifier)
			sink.TicketNeedsHuman(r.ticket.Identifier, opts.EpicName, "needs-repair", reason)
			continue
		}

		if r.built {
			// This build finished with commits ready to land; its job is
			// already queued in landJobs and the worker already woken (see
			// launch). The ticket stays in launched/everLaunched — it's
			// still this run's, just queued behind landing, not available
			// for reclaim — and completed++/sink.IterationFinished wait for
			// this same ticket's landResults outcome once the land actually
			// finishes.
			landing++
			continue
		}

		if r.parkedOnChild {
			// landOne already parked a conflict-resolution child
			// needs-repair; the parent ticket's on-disk status is still
			// claimed, not done, so this is not a completed land — no
			// completed++/sink.IterationFinished. Drop it from launched so a
			// later run's reconcile can pick the orphaned claim back up
			// (see isParked's doc: "claimed" itself is never parked, so
			// nothing else would remove it here).
			delete(launched, r.ticket.Identifier)
			continue
		}

		// landCherryPick has already written elapsed/context metrics into the
		// ticket's own frontmatter by the time results yields this outcome
		// (see writeLandedMetrics), so re-parsing off disk here is simpler
		// than threading a return value up through runIteration/finishIteration.
		landedTicket := r.ticket
		liveTotal := total
		liveDone++
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
			Cost:              landedTicket.ActualCost,
			InProgress:        active,
			Completed:         liveDone,
			Total:             liveTotal,
		})
	}

	// A canceled ctx broke the loop above before the epic was actually
	// done — report the caller's own cancellation rather than a misleading
	// EpicComplete.
	if err := ctx.Err(); err != nil {
		return err
	}

	// A drained run already announced its end via DrainComplete above — a
	// trailing EpicComplete here would be a second, contradictory chat/toast
	// (see the drained field's doc comment), so skip it for that path.
	if !drained {
		sink.EpicComplete(opts.EpicName, completed, int(d.Now().Sub(runStart).Seconds()))
	}
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
