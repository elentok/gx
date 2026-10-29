package ralphloop

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/logger"
	"github.com/elentok/gx/storecommit"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// ticketTrailerKey is the commit-message trailer mergeCherryPick stamps onto
// every merged commit (see Deps.AppendTrailer) and classifyDoneTicket's last-
// resort fallback searches for (Deps.TrailerCommitExists).
const ticketTrailerKey = "Ralph-Loop-Ticket"

// TicketTrailerKey is ticketTrailerKey for the repair verbs.
const TicketTrailerKey = ticketTrailerKey

// tokensTrailerKey and elapsedTrailerKey are the commit-message trailers
// mergeCherryPick stamps alongside ticketTrailerKey whenever the merging
// session's own metrics are available (see writeMergedMetrics) — the same
// actual_context_window/elapsed_time values written to the ticket's
// frontmatter, surfaced on the merged commit too.
const (
	tokensTrailerKey  = "Ralph-Loop-Tokens"
	elapsedTrailerKey = "Ralph-Loop-Elapsed"
	costTrailerKey    = "Ralph-Loop-Cost"
)

// ticketTrailerValue builds ticketTrailerKey's value, scoped to epicName: a
// bare ticket identifier repeats across every epic (each restarts numbering
// from 01), and TrailerCommitExists searches the whole repo history, not just
// commits under this epic. An unscoped value can match a same-numbered
// ticket from a completely unrelated epic merged long ago, misreporting a
// genuinely unmerged ticket as already present — classifyDoneTicket then
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
const defaultSmartZone = SmartZoneCeiling

// SmartZoneCeiling is the one context-token ceiling for every model; `gx agent
// context-window --fits` shares it so chaining decisions match the pause point.
const SmartZoneCeiling = 130_000

// defaultWorkerSkill is the skill each iteration invokes when
// RunOptions.Skill is unset.
const defaultWorkerSkill = "gx-implement"

// codeReviewSkill is the skill launched directly for type: code-review
// tickets instead of defaultWorkerSkill/RunOptions.Skill (see
// launchSkill). It's launched as its own fresh /command rather than
// entered from inside a gx-implement session because gx-code-review sets
// disable-model-invocation: true, which blocks an in-session Skill-tool
// hand-off but not the harness starting a session with the command
// already resolved.
const codeReviewSkill = "gx-code-review"

// verifySkill is the skill launched for type: verify tickets.
const verifySkill = "gx-verify-epic"

// launchSkill is the skill an iteration on t starts under: workerSkill,
// unless t's type routes it to its own skill.
func launchSkill(t tickets.Ticket, workerSkill string) string {
	switch {
	case t.IsCodeReview():
		return codeReviewSkill
	case t.IsVerify():
		return verifySkill
	}
	return workerSkill
}

// requiredSkills is every skill this run can launch: workerSkill always, plus
// the type-routed skill of each unfinished in-scope ticket. Done tickets and
// out-of-scope ones never launch, so a skill only they need must not fail
// preflight.
func requiredSkills(epic tickets.Epic, scope RunScope, workerSkill string) []string {
	skills := []string{workerSkill}
	for _, t := range epic.Tickets {
		if !scope.Contains(t, epic) || epic.RenderedStatus(t).Terminal() {
			continue
		}
		if skill := launchSkill(t, workerSkill); !slices.Contains(skills, skill) {
			skills = append(skills, skill)
		}
	}
	return skills
}

// AgentKind identifies the coding agent that drives an iteration.
type AgentKind string

const (
	AgentClaude AgentKind = "claude"
	AgentCodex  AgentKind = "codex"
)

// AgentFor maps a harness name to its agent kind: "codex" is Codex, anything
// else reads as Claude.
func AgentFor(harnessName string) AgentKind {
	if harnessName == string(AgentCodex) {
		return AgentCodex
	}
	return AgentClaude
}

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

// ticketAddress is the canonical "project:epic/NN" address of the ticket
// file at ticketPath (<root>/<epic>/issues/NN-<slug>.md), the only form
// iteration prompts hand to agents.
func ticketAddress(t tickets.Ticket) string {
	a, _ := tickets.AddressOfPath(t.Path, t.Identifier)
	return a.String()
}

// resolveScratchDir is the absolute ticket root for opts: ScratchDir, which
// the caller must set.
func resolveScratchDir(opts RunOptions) (string, error) {
	if opts.ScratchDir == "" {
		return "", errors.New("ralphloop: RunOptions.ScratchDir is required")
	}
	scratchDir, err := filepath.Abs(opts.ScratchDir)
	if err != nil {
		return "", fmt.Errorf("resolving scratch directory: %w", err)
	}
	return scratchDir, nil
}

// startStoreCommits starts the store commit loop when opts names a store and
// returns its stop func (a no-op otherwise).
func startStoreCommits(opts RunOptions) (stop func(), err error) {
	if opts.StoreDir == "" {
		return func() {}, nil
	}
	commits, err := storecommit.Start(opts.StoreDir, opts.StoreCommitDebounce)
	if err != nil {
		return nil, fmt.Errorf("starting store commit loop: %w", err)
	}
	return commits.Stop, nil
}

// SkillPrompt and AgentArgs expose the launch shape to the server's runner.
func SkillPrompt(agent AgentKind, skill, ticketAddr string) string {
	return skillPrompt(agent, skill, ticketAddr)
}

// extraDirs are further writable dirs for a Codex sandbox; Claude ignores them.
func AgentArgs(agent AgentKind, scratchDir, epicName, model, effort string, extraDirs ...string) []string {
	args := agentArgs(agent, scratchDir, epicName, model, effort)
	if agent != AgentCodex {
		return args
	}
	for _, d := range extraDirs {
		args = append(args, "--add-dir", d)
	}
	return args
}

func skillPrompt(agent AgentKind, skill, ticketAddr string) string {
	prefix := "/"
	if agent == AgentCodex {
		prefix = "$"
	}
	if ticketAddr == "" {
		return prefix + skill
	}
	return fmt.Sprintf("%s%s %s", prefix, skill, ticketAddr)
}

// claudeGuardSettings adds a PreToolUse hook that rejects shell file edits
// (sed -i, heredoc writes), which agents keep using despite the gx-implement
// skill forbidding them. See `gx agent guard-bash`.
const claudeGuardSettings = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"gx agent guard-bash"}]}]}}`

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
	args := []string{
		"--permission-mode", "auto", "--settings", claudeGuardSettings,
		"--disallowedTools", agentrunner.LoopDisallowedTools,
	}
	// herdr types the launch command into a shell, so keep it short: the
	// prompt goes in a file when one can be written.
	if f := harness.GXSystemPromptFile(); f != "" {
		args = append(args, "--append-system-prompt-file", f)
	} else {
		args = append(args, "--append-system-prompt", harness.GXSystemPrompt)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if effort != "" {
		args = append(args, "--effort", effort)
	}
	return args
}

// Tier names one of the three model/effort pairs configured per agent.
type Tier string

const (
	TierLight     Tier = "light"
	TierWorker    Tier = "worker"
	TierStrongest Tier = "strongest"
)

// AllTiers lists every tier, in picker order.
var AllTiers = []Tier{TierLight, TierWorker, TierStrongest}

// Valid reports whether t is one of AllTiers.
func (t Tier) Valid() bool { return slices.Contains(AllTiers, t) }

// ResolveTier is the one place that turns (agent, tier) into the model and
// effort to pass. An empty result means "pass no flag".
func ResolveTier(agents config.AgentsConfig, agent AgentKind, tier Tier) (model, effort string) {
	ac := agents.Claude
	if agent == AgentCodex {
		ac = agents.Codex
	}
	switch tier {
	case TierLight:
		return ac.Light.Model, ac.Light.Effort
	case TierStrongest:
		return ac.Strongest.Model, ac.Strongest.Effort
	}
	return ac.Model, ac.Effort
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
	Agents config.AgentsConfig
	// RetryStormLaunches is how many consecutive failed launches pause a
	// ticket as retry-exhausted; zero means the default cap.
	RetryStormLaunches int
	Skill              string // skill each iteration invokes; defaults to defaultWorkerSkill ("gx-implement") when unset
	ScratchDir         string // defaults to the repo's ticket-store project dir
	RepoDir            string // repo root passed as the herdr workspace/worktree cwd
	MaxParallel        int    // defaults to defaultMaxParallel; how many iterations run concurrently
	SmartZone          int    // defaults to defaultSmartZone; context-token ceiling before pausing an iteration
	// StoreDir, if set, is the ticket store git repo Run commits while it runs
	// (S1: the in-process loop owns store commits), debounced by
	// StoreCommitDebounce. Unset means no commits, so callers with no store
	// (tests, the read-only server) are unchanged.
	StoreDir            string
	StoreCommitDebounce time.Duration
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
	// SpinCycles pauses within SpinWindow quarantine a ticket as paused (repair)
	// "spinning" instead of re-claiming it; zero values mean the defaults.
	SpinCycles int
	SpinWindow time.Duration
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
	// already-launched iteration and the merge-queue worker have actually
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

// outcome is one iteration's or one merge's result, sent on results (a build
// goroutine) or mergeResults (the merge-queue worker) respectively.
type outcome struct {
	ticket tickets.Ticket
	err    error
	// built is true when this outcome is a build that finished with commits
	// ready to merge (ahead > 0): iteration_status: finished is already
	// written and the job is already queued in mergeJobs (see launch), so the
	// results-handling loop only needs to release this build's active slot
	// and hand the ticket to the merge queue — completed++/
	// sink.IterationFinished wait for the worker's own mergeResults outcome
	// once the merge actually finishes. Never set on a mergeResults outcome.
	built bool
	// pausedOnChild is true when this outcome is a mergeResults outcome whose
	// cherry-pick hit errConflictResolutionUnresolved: the ticket was paused
	// paused (repair) on a conflict-resolution child, not merged — it stays
	// claimed on disk. err is nil on this path (pausing already handled the
	// failure), so without this flag the results loop can't tell it apart
	// from a real completed merge. Never set on a results (build) outcome.
	pausedOnChild bool
	// childReason names the conflict-resolution child and why it failed, on a
	// pausedOnChild outcome; the server pauses the parent with it.
	childReason string
	// mergeDeferred is set by mergeOne when the merge lock was held or a human's
	// conflict marker is pending; runMergeQueue re-queues the job and never
	// forwards the outcome to mergeResults.
	mergeDeferred bool
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
	workerModel, workerEffort := ResolveTier(opts.Agents, agent, TierWorker)
	runStart := d.Now()

	scratchDir, err := resolveScratchDir(opts)
	if err != nil {
		return err
	}
	stopCommits, err := startStoreCommits(opts)
	if err != nil {
		return err
	}
	defer stopCommits()
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
		var errs []error
		for _, s := range requiredSkills(*initial, scope, skill) {
			if err := d.VerifySkill(agent, s); err != nil {
				errs = append(errs, fmt.Errorf("preflighting %s skill %q: %w", agent, s, err))
			}
		}
		if err := errors.Join(errs...); err != nil {
			return err
		}
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
	// ticket, and also guards the merge-queue worker's own eligibility scan
	// and mergeJobs map (see mergequeue.go) — the same shape of operation.
	// worktreeMu guards every `git worktree add`/`git worktree remove`
	// against RepoDir, since git's own worktree administrative files aren't
	// safe under concurrent add/remove on the same repo. sink is safe for
	// concurrent use on its own (see EventSink), since a paused iteration
	// reports its pause/resume from its own goroutine. The merge-queue
	// worker's single-goroutine-ness is what now serializes cherry-pick
	// merges onto the feature worktree — no lock needed for that anymore.
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
	mergeResults := make(chan outcome)

	// mergeJobs holds the live-process handoff data (see mergeJob's doc) for
	// every ticket a build goroutine has finished but the merge-queue worker
	// hasn't yet merged, keyed by ticket identifier. Written by launch's
	// goroutine before it sends a built:true outcome, read and deleted by the
	// merge-queue worker once it starts merging that ticket. mergeJobsMu guards
	// it since the two run concurrently.
	mergeJobs := make(map[string]mergeJob)
	var mergeJobsMu sync.Mutex

	// mergeWake nudges the merge-queue worker to rescan immediately after a
	// fresh job is queued, rather than only on its own idle poll — a buffered
	// size-1 channel so a wake that arrives while the worker is already
	// mid-pass isn't lost, just coalesced into the next one.
	mergeWake := make(chan struct{}, 1)
	wakeMergeQueue := func() {
		select {
		case mergeWake <- struct{}{}:
		default:
		}
	}
	mergeQueueDone := make(chan struct{})
	defer close(mergeQueueDone)

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
			ticket, err = firstNonSpinning(opts, d.Now(), sink, scratchDir, wtDir, frontier, func(t tickets.Ticket) bool {
				return launched[t.Identifier]
			})
			if err != nil {
				return err
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
				reattach = resumeReattachable(d, opts.EpicName, agent, wtDir, ticket)
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
		// so there's nothing useful to log. A failed log is only debug-logged:
		// the ticket is already claimed, so failing the run here would strand
		// it claimed with no iteration behind it.
		if scanned != nil {
			if logErr := logSchedulerScan(scratchDir, opts.EpicName, agent, scanDecisions(*scanned, scope, frontier, ticket)); logErr != nil {
				logger.Debug("logging scheduler scan: %v\n", logErr)
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
				RepoDir:         opts.RepoDir,
				WorktreeDir:     wtDir,
				FeatureWorktree: featurePath,
				FeatureBranch:   opts.EpicName,
				Agent:           agent,
				Model:           workerModel,
				Effort:          workerEffort,
				Skill:           skill,
				Ticket:          ticket,
				ScratchDir:      scratchDir,
				WorktreeLock:    &worktreeMu,
				SmartZone:       smartZone,
				Gate:            gate,
				Sink:            sink,
				verifyCap:       new(verifyCapState),
			}
			var err error
			if reattach {
				err = reattachIteration(d, params)
			} else {
				err = runIteration(d, params)
			}
			var built *builtAwaitingMergeError
			if errors.As(err, &built) {
				mergeJobsMu.Lock()
				mergeJobs[ticket.Identifier] = built.job
				mergeJobsMu.Unlock()
				wakeMergeQueue()
				results <- outcome{ticket: ticket, built: true}
				return
			}
			results <- outcome{ticket: ticket, err: err}
		}()
	}

	completed := 0
	active := 0
	// merging tracks builds that finished (ahead > 0, iteration_status:
	// finished written) but haven't yet merged — queued in mergeJobs, waiting
	// on the merge-queue worker. Incremented when a results outcome arrives
	// with built == true, decremented when the worker reports that ticket's
	// merging over mergeResults. Both the exit condition and the idle/
	// deadlock branch below gate on active == 0 && merging == 0: a ticket
	// queued behind an unmerged merge is neither done nor a stall a human
	// needs to clear.
	merging := 0
	// liveDone tracks the epic-wide done count: seeded from disk once here so
	// a resumed run reports what's true of the epic even though this run's
	// own completed count starts back at zero, then incremented by exactly
	// one per merged outcome this loop empties below (never re-derived from a
	// live disk rescan — two near-simultaneous merges can both already be
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
		Paths:        reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: featurePath, WorktreeDir: wtDir, RepoDir: opts.RepoDir},
		Agent:        agent,
		Model:        workerModel,
		Effort:       workerEffort,
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
	// A ticket found paused (repair) at startup deliberately does not pause
	// the gate: it's human-clearable, so the run schedules every other
	// runnable ticket first and only pauses on it once nothing is runnable.
	for _, ticket := range reattached {
		launch(ticket, true)
		active++
		launched[ticket.Identifier] = true
		everLaunched[ticket.Identifier] = true
	}

	// The merge-queue worker is the sole serializer of cherry-pick merges
	// for this Run call — replacing featureMu — and runs as a single
	// goroutine decoupled from the build goroutines above, so a build's
	// active slot never waits on a merge (including up to 30 minutes of
	// conflict resolution). It requires no persisted state of its own: the
	// in-memory mergeJobs map (pane/tab/sessionID/path) is what survives the
	// build-goroutine-to-worker handoff, and is also the sole source of
	// merging eligibility (see runMergeQueue).
	go runMergeQueue(d, mergeQueueParams{
		RepoDir:         opts.RepoDir,
		WorktreeDir:     wtDir,
		FeatureWorktree: featurePath,
		FeatureBranch:   opts.EpicName,
		Agent:           agent,
		Model:           workerModel,
		Effort:          workerEffort,
		Skill:           skill,
		ScratchDir:      scratchDir,
		WorktreeLock:    &worktreeMu,
		SmartZone:       smartZone,
		Gate:            gate,
		Sink:            sink,
	}, mergeJobs, &mergeJobsMu, mergeWake, mergeQueueDone, mergeResults)

	// paused records that the run has already announced this pause, so the
	// poll loop below notifies once per pause rather than once per tick. It
	// resets the moment anything is running again, so a run that resumes and
	// later pauses on a different ticket announces that pause too.
	paused := false

	for {
		epic, err := loadNamedEpic(scratchDir, opts.EpicName)
		if err != nil {
			return err
		}
		// resumeAnswered runs on every pass through this loop — the initial
		// entry, each completed iteration, and (via the select below) every
		// pause-poll tick even while siblings are still running — reusing the
		// epic load just above rather than scanning again on its own. See
		// resumeAnswered's doc for why a live, unblocked pane is what makes
		// this safe with no ownership check.
		if err := resumeAnswered(d, opts.EpicName, wtDir, agent, scope, *epic, d.Now()); err != nil {
			return err
		}
		if scope.AllDone(*epic) && active == 0 && merging == 0 {
			if allDone(*epic) {
				if err := tickets.StampEpicCompleted(scratchDir, opts.EpicName, d.Now()); err != nil {
					return fmt.Errorf("stamping epic %q completed_at: %w", opts.EpicName, err)
				}
			}
			break
		}
		// A canceled ctx with nothing left in flight is the shutdown path's
		// own exit: nothing more to empty, so stop here rather than fall
		// through to claiming (guarded below) or pausing (which would just
		// block again on the very thing being canceled).
		if ctx.Err() != nil && active == 0 && merging == 0 {
			break
		}

		if active == 0 {
			acquirePermit()
		}
		// Once canceled, stop recruiting new work — but every iteration and
		// merge already in flight keeps running to completion below, so their
		// goroutines are emptied rather than orphaned on a channel nobody
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

		if active == 0 && merging == 0 {
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
			// claimNext may have quarantined a ticket after this pass loaded
			// epic, so judge deadlock against the statuses on disk now.
			if fresh, err := loadNamedEpic(scratchDir, opts.EpicName); err == nil && fresh != nil {
				epic = fresh
			}
			stalled := stalledTickets(*epic, scope)
			if len(stalled) == 0 {
				logDeadlocked(scratchDir, opts.EpicName, events.BlockedCycle, "nothing runnable and nothing a person could clear")
				return fmt.Errorf("epic %q is deadlocked: no runnable tickets left, none done, and none a human could clear; check for a dependency cycle or a bad status", opts.EpicName)
			}
			if !paused {
				if epic.AllAwaitAnswer(stalled) {
					logWaitingForAnswer(scratchDir, opts.EpicName, "every remaining ticket waits on an answer")
				} else {
					logDeadlocked(scratchDir, opts.EpicName, events.AllPaused, "every remaining ticket is paused")
				}
				stalledForSink := make([]StalledTicket, len(stalled))
				for i, t := range stalled {
					stalledForSink[i] = StalledTicket{
						Identifier:   t.Identifier,
						Reattachable: everLaunched[t.Identifier] && clearablePausedTicket(d, opts.EpicName, wtDir, agent, *epic, t),
					}
				}
				sink.EpicPaused(opts.EpicName, stalledForSink)
				paused = true
			}
			// No timeout: the notification has already fired, and the pause
			// ends when a human clears one of the stalled tickets, which the
			// next pass picks up off disk. gate.WakePaused() lets an operator
			// cut the wait short cosmetically; ctx.Done() lets a caller stop
			// the run outright.
			select {
			case <-d.PauseTimer(pausedPollInterval):
			case <-gate.PauseWake():
			case <-ctx.Done():
			}
			continue
		}
		paused = false

		// With a sibling still running (active > 0, we're past the branch
		// above), a plain receive would block until that sibling finishes even
		// if some other ticket is already sitting paused (answer) with its pane
		// answered — the ticket-15 gap. Selecting the same pause-timer seam the
		// idle branch above already polls on catches that case: each tick
		// loops back to the top, where the epic reload and resumeAnswered
		// pick it up. Gated on hasLivePausedTicket rather than "something is
		// paused" at all: a paused (answer) ticket with no live pane can only be
		// cleared by a person editing the file (already caught by the next
		// ordinary reload), and polling for it here would race an
		// always-ready pause-timer fake against the sibling's results send,
		// starving it out forever.
		// The select below always includes mergeResults alongside results —
		// harmless even when merging == 0, since nothing sends on it until
		// the worker has a job — so a merge finishing never has to wait for a
		// sibling build's own results send.
		var r outcome
		var fromMerge bool
		if active > 0 && hasLivePausedTicket(d, opts.EpicName, wtDir, agent, scope, *epic) {
			select {
			case r = <-results:
			case r = <-mergeResults:
				fromMerge = true
			case <-d.PauseTimer(pausedPollInterval):
				continue
			}
		} else {
			select {
			case r = <-results:
			case r = <-mergeResults:
				fromMerge = true
			}
		}
		if fromMerge {
			merging--
		} else {
			active--
		}
		if r.err != nil {
			// A single iteration (or merge) erroring out (a herdr/git hiccup,
			// an unexpected agent-wait failure, a cherry-pick conflict that
			// couldn't be resolved, ...) shouldn't take down the whole
			// epic's run — that leaves every other in-flight ticket's
			// live-event stream dead too, with no way to recover short of
			// restarting the loop. Flag just this ticket paused (repair)
			// (out of the frontier, so never reclaimed until a human clears
			// it) and keep scheduling the rest.
			kind := events.IterationError
			reason := r.err.Error()
			var lf *launchFailure
			if errors.As(r.err, &lf) {
				kind, reason = retryStormKind(scratchDir, opts.EpicName, r.ticket.Identifier, opts.RetryStormLaunches, lf.Kind, reason)
			}
			pause(sink, pauseRequest{
				ScratchDir: scratchDir,
				EpicName:   opts.EpicName,
				Ticket:     r.ticket.Identifier,
				Path:       r.ticket.Path,
				Type:       events.Paused,
				Kind:       kind,
				Reason:     reason,
				Repair: schema.FaultState{
					Label:    iterLabel(opts.EpicName, r.ticket.Identifier),
					Branch:   iterBranch(opts.EpicName, r.ticket.Identifier),
					Worktree: iterationWorktreePath(wtDir, opts.EpicName, r.ticket.Identifier),
				},
			})
			delete(launched, r.ticket.Identifier)
			continue
		}

		if r.built {
			// This build finished with commits ready to merge; its job is
			// already queued in mergeJobs and the worker already woken (see
			// launch). The ticket stays in launched/everLaunched — it's
			// still this run's, just queued behind merging, not available
			// for reclaim — and completed++/sink.IterationFinished wait for
			// this same ticket's mergeResults outcome once the merge actually
			// finishes.
			merging++
			continue
		}

		if r.pausedOnChild {
			// mergeOne already paused a conflict-resolution child
			// paused (repair); the parent ticket's on-disk status is still
			// claimed, not done, so this is not a completed merge — no
			// completed++/sink.IterationFinished. Drop it from launched so a
			// later run's reconcile can pick the orphaned claim back up
			// (see tickets.Epic.IsPaused: "claimed" itself is never paused, so
			// nothing else would remove it here).
			delete(launched, r.ticket.Identifier)
			continue
		}

		// mergeCherryPick has already written elapsed/context metrics into the
		// ticket's own frontmatter by the time results yields this outcome
		// (see writeMergedMetrics), so re-parsing off disk here is simpler
		// than threading a return value up through runIteration/finishIteration.
		mergedTicket := r.ticket
		liveTotal := total
		paused := false
		if mergedEpic, err := loadNamedEpic(scratchDir, opts.EpicName); err != nil {
			return err
		} else if mergedEpic != nil {
			liveTotal = scope.TotalCount(*mergedEpic)
			for _, t := range mergedEpic.Tickets {
				if t.Identifier == r.ticket.Identifier {
					mergedTicket = t
					// An iteration that ended stalled (paused (answer), most
					// commonly) is no longer launched, so a human clearing
					// its status puts it back in the frontier for this run
					// instead of only for the next one.
					if mergedEpic.IsPaused(t) {
						paused = true
						delete(launched, t.Identifier)
					}
					break
				}
			}
		}

		if paused {
			// A paused outcome (paused (answer)/paused (repair), no commits) already
			// got its own notification via sink.TicketNeedsHuman (fired from
			// iteration.go/waitforfinish.go) — it never merged, so it must not
			// also count toward completed or emit a contradictory "done"
			// sink.IterationFinished, mirroring the r.pausedOnChild early-exit
			// above.
			continue
		}

		liveDone++
		completed++
		sink.IterationFinished(mergedTicket, opts.EpicName, IterationStats{
			ElapsedSeconds:    mergedTicket.ElapsedTime,
			PeakContextTokens: mergedTicket.ActualContextWindow,
			Cost:              mergedTicket.ActualCost,
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
		isClaimed := claimed.Path != "" && t.Path == claimed.Path
		decisions = append(decisions, TicketVerdict(epic, scope, t, isClaimed, inFrontier[t.Path]))
	}
	return decisions
}

// TicketVerdict is the scheduler's single per-ticket eligibility decision:
// why t was or wasn't claimed this pass, with the reason a scan line reports.
// The first matching rule wins, so the order is the precedence.
func TicketVerdict(epic tickets.Epic, scope RunScope, t tickets.Ticket, claimed, inFrontier bool) ScanDecision {
	status := epic.RenderedStatus(t)
	d := ScanDecision{Ticket: t.Identifier, Status: status.Word()}
	switch {
	case claimed:
		d.Decision = DecisionClaimed
	case !scope.Contains(t, epic):
		d.Decision = DecisionOutOfScope
	case status.Terminal():
		d.Decision = DecisionDone
	case status == tickets.StatusError:
		d.Decision = DecisionError
		d.Reason = cmp.Or(t.ReadErr, t.GraphErr, t.BlockedByErr, "unrecognized status "+t.Status)
	case epic.IsPaused(t):
		d.Decision = DecisionStalled
	case inFrontier:
		d.Decision = DecisionFrontier
	case status == tickets.StatusBlocked:
		d.Decision = DecisionBlocked
		d.Reason = strings.Join(epic.UnresolvedBlockers(t), ", ")
	default:
		d.Decision = DecisionUnclaimed
	}
	return d
}

// AllDone is allDone for the server's runner.
func AllDone(e tickets.Epic) bool { return allDone(e) }

// allDone reports whether every ticket in e is done — the run's one exit
// condition. A stalled ticket (see tickets.Epic.IsPaused) is deliberately not an
// exit condition: the run pauses on it instead, so the agent's question is
// still answerable in its own pane.
func allDone(e tickets.Epic) bool {
	if len(e.Tickets) == 0 {
		return false
	}
	for _, t := range e.Tickets {
		if !e.RenderedStatus(t).Terminal() {
			return false
		}
	}
	return true
}

// stalledTickets lists every in-scope, human-clearable ticket in e, in file
// order — what a pause notification names as the thing waiting on a person.
func stalledTickets(e tickets.Epic, scope RunScope) []tickets.Ticket {
	var stalled []tickets.Ticket
	for _, t := range e.Tickets {
		if scope.Contains(t, e) && e.IsPaused(t) {
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
