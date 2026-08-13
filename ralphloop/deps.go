package ralphloop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/transcript"
)

// Deps are the external side-effecting operations the loop drives: herdr
// socket-API calls, git worktree/cherry-picking, and the smart-zone
// guardrail's transcript/pause-signal reads. DefaultDeps wires them to the
// real herdr/git/transcript packages; tests substitute fakes so the
// orchestration in loop.go can run without a live herdr server, git
// process, or real Claude Code transcripts.
type Deps struct {
	// PreflightAgent verifies that the selected agent can be launched before
	// the loop creates Herdr state or claims a ticket.
	PreflightAgent func(agent AgentKind) error
	// VerifySkill confirms opts.Skill is installed for agent before the loop
	// creates Herdr state or claims a ticket — like PreflightAgent, a missing
	// skill is otherwise only discovered mid-iteration, after the ticket is
	// already claimed and the agent has been prompted with a skill invocation
	// it can't resolve.
	VerifySkill           func(agent AgentKind, skill string) error
	FindOrCreateWorkspace func(label, cwd string) (string, error)
	// FindWorkspace looks up an epic's herdr workspace without creating one,
	// used by the restart-recovery reattach scan (see ScanForReattachable),
	// which must never bring a workspace into existence just to discover it
	// doesn't have one.
	FindWorkspace func(label string) (string, error)
	// WorktreeDir returns the directory linked worktrees for repoDir's repo
	// are created in (see git.Repo.LinkedWorktreeDir).
	WorktreeDir func(repoDir string) (string, error)
	// AddWorktree creates a plain git worktree at path on a new branch,
	// starting at base (a ref or commit hash; "" for the repo's HEAD) — or,
	// if branch already exists, attaches the worktree to that branch instead
	// (base is then ignored), for a resume that reattaches to the iteration
	// branch a prior park left intact. A no-op if path already exists, so a
	// resumed Run can call it again for a worktree a prior invocation already
	// created.
	AddWorktree func(repoDir, path, branch, base string) error
	// RemoveWorktree removes the git worktree checked out at path.
	RemoveWorktree func(repoDir, path string, force bool) error
	// DeleteBranch force-deletes an iteration's now-redundant branch once its
	// commits have landed on the feature branch (as different hashes, via
	// cherry-pick — never merged, so a non-force delete would refuse it).
	DeleteBranch  func(repoDir, branch string) error
	TabCreate     func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error)
	TabClose      func(tabID string) error
	TabList       func(workspaceID string) ([]herdr.Tab, error)
	AgentStart    func(opts herdr.AgentStartOptions) (herdr.Agent, error)
	AgentPrompt   func(opts herdr.AgentPromptOptions) (herdr.Agent, error)
	AgentGet      func(target string) (herdr.Agent, error)
	AgentWait     func(opts herdr.AgentWaitOptions) (herdr.Agent, error)
	AgentSendKeys func(target string, keys ...string) error
	// AgentRead reads pane's terminal output, used to confirm a submitted
	// prompt has actually rendered rather than still sitting unsubmitted (see
	// confirmCompactSubmitted).
	AgentRead            func(target string, opts herdr.AgentReadOptions) (string, error)
	RevParse             func(dir, ref string) (string, error)
	MergeBase            func(dir, refA, refB string) (string, error)
	CommitsAhead         func(dir, fromExclusive, toRef string) (int, error)
	CherryPickRange      func(dir, fromExclusive, toInclusive string) error
	CherryPickInProgress func(dir string) (bool, error)
	// AbortCherryPick clears sequencer state in the shared feature worktree;
	// the durable source iteration branch is unaffected.
	AbortCherryPick func(dir string) error
	// IsAncestor reports whether ancestor is reachable from descendant, used
	// by startup reconciliation to confirm a done ticket's recorded landed
	// commit (Event.SHA) is still on the feature branch.
	IsAncestor func(dir, ancestor, descendant string) (bool, error)
	// PatchesApplied reports whether a done ticket's iteration commits
	// (base..branch) are already patch-equivalent to commits on the feature
	// branch, used by startup reconciliation as a fallback when IsAncestor
	// says a recorded landed commit isn't reachable — which also happens
	// harmlessly whenever the feature branch was rebased after landing it,
	// not just when the commit is genuinely missing.
	PatchesApplied func(dir, upstream, base, branch string) (bool, error)
	// AppendTrailers amends HEAD's commit message to add one or more
	// ticket-identifying/metrics trailers in a single amend, stamped onto
	// every landed cherry-pick so classifyDoneTicket can still find the
	// ticket trailer later even if a subsequent rebase-plus-manual-conflict-
	// resolution changes the commit's hash and patch-id both.
	AppendTrailers func(dir string, trailers ...git.Trailer) error
	// WorktreeExists reports whether an iteration worktree still exists at
	// path, used by startup reconciliation to detect leftover state a crash
	// left uncleaned.
	WorktreeExists func(path string) (bool, error)
	// InstallDeps detects the package manager of a freshly created iteration
	// worktree at path (from marker files at its root) and runs its
	// non-interactive install/sync command before the agent is launched.
	// command is the command run, joined with spaces ("" if no marker
	// matched, which is a silent no-op rather than an error).
	InstallDeps func(path string) (command string, err error)

	// ReadOccupancy returns the current context occupancy for the Claude
	// Code session launched in cwd, or ok=false if its transcript has no
	// assistant turn yet.
	ReadOccupancy func(cwd, sessionID string) (occupancy int, ok bool, err error)
	// ReadOccupancyReading returns the same occupancy plus whether a
	// compaction boundary has landed since the turn it came from. Only the
	// smart-zone breach check wants that second answer (see
	// smartZoneOccupancy); every other consumer reads ReadOccupancy, which
	// keeps reporting the number regardless.
	ReadOccupancyReading func(cwd, sessionID string) (transcript.OccupancyReading, error)
	// ReadCompactions returns how many compaction boundaries the Claude Code
	// session launched in cwd hit, or ok=false if its transcript can't be
	// found yet.
	ReadCompactions func(cwd, sessionID string) (count int, ok bool, err error)
	// ReadCompactionsAfter returns how many of those boundaries were written
	// strictly after since. The compact-completion gate needs this whenever it
	// has no trustworthy pre-"/compact" count to compare against: a total count
	// read after the boundary landed already includes it, so no comparison
	// against it can prove the compaction happened (see stickyBaseline).
	ReadCompactionsAfter func(cwd, sessionID string, since time.Time) (count int, ok bool, err error)
	// ReadBackgroundTasks returns the Claude Code session's outstanding
	// backgrounded-shell-command markers, used to gate confirmFinished against
	// a pane that looks idle while a background task it started is still
	// running (see waitForBackgroundTasks). Codex has no equivalent transcript
	// signal, so callers only ever invoke this for AgentClaude.
	ReadBackgroundTasks func(cwd, sessionID string) (transcript.BackgroundTaskReading, error)
	// ReadCodexContext returns the latest context-token count for the Codex
	// session launched in cwd, or ok=false until its local session data is
	// complete enough to identify that worktree and session.
	ReadCodexContext func(cwd, sessionID string) (tokens int, ok bool, err error)
	// VerifyCodexSession confirms that sessionID's rollout metadata belongs to cwd.
	VerifyCodexSession func(cwd, sessionID string) (ok bool, err error)
	// ReadCodexRateLimit returns an exhausted Codex quota for the session
	// launched in cwd, or ok=false when its session data is incomplete or no
	// quota is exhausted.
	ReadCodexRateLimit func(cwd, sessionID string) (limit codexsession.RateLimit, ok bool, err error)
	// ReadPaneRecent returns pane's recent terminal output, used to detect a
	// Claude usage/session rate-limit message.
	ReadPaneRecent func(pane string) (string, error)
	// Sleep is how a paused loop waits between poll checks.
	Sleep func(time.Duration)
	// Now returns the current time, injectable so a rate-limit reset
	// deadline can be tested without a real wall-clock wait.
	Now func() time.Time
	// ParkTimer returns a channel that fires once the given duration has
	// elapsed — one poll of a parked run's wait. It is a channel rather than
	// a blocking sleep so the park can select on an operator's cosmetic wake
	// at the same time without a goroutine per poll, and it is injectable so
	// park tests drive polling with a timer that is ready immediately.
	ParkTimer func(time.Duration) <-chan time.Time
}

// DefaultDeps wires Deps to the real herdr, git, and transcript packages.
func DefaultDeps() Deps {
	return DefaultDepsWithOverrides(DepsOverrides{})
}

// DepsOverrides substitutes the home directory(ies) and PATH the in-process
// reads below resolve against, in place of the process's real HOME/
// CODEX_HOME/PATH. Those reads — codexsession.codexHome, transcript.Path,
// verifySkill's os.UserHomeDir, and preflightAgent/InstallDependencies's
// exec.LookPath — happen inside this Go process rather than a spawned
// subprocess, so exec.Cmd.Env can't influence them; the only prior way to
// steer them per-test was t.Setenv, which mutates process env and so can't
// be used safely across parallel tests. A zero field keeps that field's real
// env resolution exactly as before.
type DepsOverrides struct {
	// Home, when non-empty, overrides os.UserHomeDir() for verifySkill's
	// skill-file lookup and transcript.Path's transcript-file lookup.
	Home string
	// CodexHome, when non-empty, overrides codexsession's CODEX_HOME/
	// UserHomeDir resolution directly (the resolved ~/.codex-equivalent
	// directory, not $HOME) for Deps.ReadCodexContext/ReadCodexRateLimit.
	CodexHome string
	// Path, when non-empty, overrides os.Getenv("PATH") for codex/herdr/
	// package-manager executable lookups (PreflightAgent, InstallDeps).
	Path string
}

// DefaultDepsWithOverrides is DefaultDeps with overrides applied to the
// in-process env reads described on DepsOverrides.
func DefaultDepsWithOverrides(overrides DepsOverrides) Deps {
	return Deps{
		PreflightAgent: func(agent AgentKind) error {
			return preflightAgentWith(agent, lookPathFor(overrides.Path), commandOutput)
		},
		VerifySkill: func(agent AgentKind, skill string) error {
			return verifySkillWith(agent, skill, userHomeDirFor(overrides.Home), os.Stat)
		},
		AgentGet: herdr.AgentGet,
		VerifyCodexSession: codexHomeFn(overrides.CodexHome,
			codexsession.VerifyIdentity,
			func(cwd, sessionID string) (bool, error) {
				return codexsession.VerifyIdentityIn(overrides.CodexHome, cwd, sessionID)
			},
		),
		FindOrCreateWorkspace: herdr.EnsureWorkspace,
		FindWorkspace:         herdr.FindWorkspace,
		WorktreeDir:           worktreeDir,
		AddWorktree:           addWorktree,
		RemoveWorktree:        removeWorktree,
		DeleteBranch:          deleteBranch,
		TabCreate:             herdr.TabCreate,
		TabClose:              herdr.TabClose,
		TabList:               herdr.TabList,
		AgentStart:            herdr.AgentStart,
		AgentPrompt:           promptWithNudge(herdr.AgentPrompt, herdr.AgentSendKeys, herdr.AgentWait, herdr.AgentRead, time.Now),
		AgentWait:             herdr.AgentWait,
		AgentSendKeys:         herdr.AgentSendKeys,
		AgentRead:             herdr.AgentRead,
		RevParse:              git.RevParse,
		MergeBase:             git.MergeBase,
		CommitsAhead:          git.CommitsAhead,
		CherryPickRange:       git.CherryPickRange,
		CherryPickInProgress:  git.CherryPickInProgress,
		AbortCherryPick:       git.AbortCherryPick,
		IsAncestor:            git.IsAncestor,
		PatchesApplied:        git.PatchesApplied,
		AppendTrailers:        git.AppendTrailers,
		WorktreeExists:        worktreeExists,
		InstallDeps: func(path string) (string, error) {
			return installDependenciesWith(path, overrides.Path)
		},
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) {
			path, err := transcriptPath(overrides.Home, cwd, sessionID)
			if err != nil {
				return 0, false, err
			}
			usage, ok, err := transcript.LastAssistantUsage(path)
			if err != nil || !ok {
				return 0, ok, err
			}
			return usage.Occupancy(), true, nil
		},
		ReadOccupancyReading: func(cwd, sessionID string) (transcript.OccupancyReading, error) {
			path, err := transcriptPath(overrides.Home, cwd, sessionID)
			if err != nil {
				return transcript.OccupancyReading{}, err
			}
			return transcript.ReadOccupancy(path)
		},
		ReadCompactions: func(cwd, sessionID string) (int, bool, error) {
			path, err := transcriptPath(overrides.Home, cwd, sessionID)
			if err != nil {
				return 0, false, err
			}
			lines, ok, err := transcript.ReadAll(path)
			if err != nil || !ok {
				return 0, ok, err
			}
			return transcript.CountCompactions(lines), true, nil
		},
		ReadCompactionsAfter: func(cwd, sessionID string, since time.Time) (int, bool, error) {
			path, err := transcriptPath(overrides.Home, cwd, sessionID)
			if err != nil {
				return 0, false, err
			}
			lines, ok, err := transcript.ReadAll(path)
			if err != nil || !ok {
				return 0, ok, err
			}
			return transcript.CountCompactionsAfter(lines, since), true, nil
		},
		ReadBackgroundTasks: func(cwd, sessionID string) (transcript.BackgroundTaskReading, error) {
			path, err := transcriptPath(overrides.Home, cwd, sessionID)
			if err != nil {
				return transcript.BackgroundTaskReading{}, err
			}
			return transcript.ReadBackgroundTasks(path, backgroundTaskAgedOutCap, time.Now())
		},
		ReadCodexContext: codexHomeFn(overrides.CodexHome,
			codexsession.LastContextTokens,
			func(cwd, sessionID string) (int, bool, error) {
				return codexsession.LastContextTokensIn(overrides.CodexHome, cwd, sessionID)
			},
		),
		ReadCodexRateLimit: codexHomeFn(overrides.CodexHome,
			codexsession.LastRateLimit,
			func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
				return codexsession.LastRateLimitIn(overrides.CodexHome, cwd, sessionID)
			},
		),
		ReadPaneRecent: defaultReadPaneRecent,
		Sleep:          time.Sleep,
		Now:            time.Now,
		ParkTimer:      time.After,
	}
}

// transcriptPath resolves cwd/sessionID's transcript file path — home when
// set, real os.UserHomeDir() otherwise (transcript.Path's own behavior) — so
// ReadOccupancy/ReadOccupancyReading/ReadCompactions/ReadCompactionsAfter
// share one home-resolution branch instead of repeating it four times.
func transcriptPath(home, cwd, sessionID string) (string, error) {
	if home != "" {
		return transcript.PathIn(home, cwd, sessionID), nil
	}
	return transcript.Path(cwd, sessionID)
}

// codexHomeFn returns real when codexHome is empty (codexsession's own
// CODEX_HOME/UserHomeDir resolution) or override otherwise (codexHome pinned
// explicitly), mirroring transcriptPath's home-resolution branch for
// VerifyCodexSession/ReadCodexContext/ReadCodexRateLimit's CodexHome
// override, each of which pairs a differently-shaped real/In function.
func codexHomeFn[F any](codexHome string, real, override F) F {
	if codexHome == "" {
		return real
	}
	return override
}

// lookPathFor returns exec.LookPath when pathEnv is empty (real process
// PATH), or a lookup against the explicit pathEnv string otherwise.
func lookPathFor(pathEnv string) func(string) (string, error) {
	if pathEnv == "" {
		return exec.LookPath
	}
	return func(file string) (string, error) { return lookPathIn(file, pathEnv) }
}

// userHomeDirFor returns os.UserHomeDir when home is empty (real process
// HOME), or a constant home otherwise.
func userHomeDirFor(home string) func() (string, error) {
	if home == "" {
		return os.UserHomeDir
	}
	return func() (string, error) { return home, nil }
}

// lookPathIn resolves file against the PATH-style list pathEnv, the same
// resolution exec.LookPath performs against the process's real PATH — used
// so preflightAgentWith and InstallDependencies can be pointed at an
// explicit PATH string per call instead of exec.LookPath's process-PATH
// read, which no per-test override can reach without mutating process env.
func lookPathIn(file, pathEnv string) (string, error) {
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, file)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode()&0111 != 0 {
			return candidate, nil
		}
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

func commandOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func preflightAgentWith(
	agent AgentKind,
	lookPath func(string) (string, error),
	commandOutput func(string, ...string) ([]byte, error),
) error {
	if agent != AgentCodex {
		return nil
	}
	codexPath, err := lookPath("codex")
	if err != nil {
		return errors.New("codex executable not found in PATH; install Codex or add it to PATH")
	}
	loginStatus, statusErr := commandOutput(codexPath, "login", "status")
	if isCodexUnauthenticated(string(loginStatus)) {
		return errors.New("codex is not authenticated; run `codex login`")
	}
	if statusErr != nil {
		return fmt.Errorf("could not verify Codex authentication: %w", statusErr)
	}

	herdrPath, err := lookPath("herdr")
	if err != nil {
		return errors.New("Herdr executable not found in PATH; install or upgrade Herdr with Codex integration")
	}
	help, err := commandOutput(herdrPath, "agent", "start", "--help")
	if err != nil {
		return fmt.Errorf("could not verify Herdr Codex integration; upgrade Herdr: %w", err)
	}

	const valuesPrefix = "[possible values:"
	start := strings.Index(string(help), valuesPrefix)
	if start >= 0 {
		values := string(help)[start+len(valuesPrefix):]
		if end := strings.IndexByte(values, ']'); end >= 0 {
			for _, value := range strings.Split(values[:end], ",") {
				if strings.TrimSpace(value) == string(AgentCodex) {
					return nil
				}
			}
		}
	}
	return errors.New("installed Herdr does not support Codex agents; upgrade Herdr")
}

// isCodexUnauthenticated reports whether codex login status's output
// indicates Codex has no valid credentials — distinct from the executable
// simply being missing (a lookPath failure) or Herdr's Codex integration
// being missing/incompatible (a herdrPath/help-output failure), both checked
// separately above so an operator sees the actual cause instead of a single
// generic "Codex launch failed".
func isCodexUnauthenticated(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "not logged in") || strings.Contains(lower, "not authenticated")
}

// verifySkillWith confirms skill is installed where agent resolves its
// "/skill"/"$skill" invocation from: Claude Code skills live under
// ~/.claude/skills/<skill>/SKILL.md, Codex custom prompts under
// ~/.codex/prompts/<skill>.md. lookPath/commandOutput-style injected
// userHomeDir/stat keep this testable without touching the real filesystem.
func verifySkillWith(
	agent AgentKind,
	skill string,
	userHomeDir func() (string, error),
	stat func(string) (os.FileInfo, error),
) error {
	home, err := userHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory to verify skill %q: %w", skill, err)
	}
	path := filepath.Join(home, ".claude", "skills", skill, "SKILL.md")
	if agent == AgentCodex {
		path = filepath.Join(home, ".codex", "prompts", skill+".md")
	}
	if _, err := stat(path); err != nil {
		return fmt.Errorf("skill %q not found at %s; install it or pass a different --skill", skill, path)
	}
	return nil
}

// promptNudgeGraceMs bounds each of promptWithNudge's submit/re-wait attempts.
// promptMaxNudges caps how many bare-Enter nudges it sends before giving up
// and returning the underlying poll-timeout error. promptMaxRetypes caps how
// many times it falls back to resubmitting opts.Text in full (see
// errStuckSubmission's doc comment) before giving up early instead of
// spending the rest of promptMaxNudges on nudges that can't help either.
//
// 4_000 was too tight for a cold-started agent to reach "working" under
// concurrent load — observed live as a hard failure quoting the literal
// `--timeout 4000` this constant produces, even with plenty of budget left
// on the caller's own overall deadline (budgetMs caps every individual
// attempt to this constant regardless of that deadline). Raised to give
// cold starts headroom under contention.
const (
	promptNudgeGraceMs = 45_000
	promptMaxNudges    = 3
	promptMaxRetypes   = 2
)

// errStuckSubmission is promptWithNudge's sentinel for a pane that shows no
// change at all across promptMaxRetypes full resubmissions of opts.Text —
// distinct from the underlying poll-timeout error so a caller (runIteration)
// can tell "the agent is slow to start, a bare nudge might still land" apart
// from "this pane genuinely never received anything and retyping the same
// text into it again won't help either," and retry against a fresh pane
// instead.
var errStuckSubmission = errors.New("submission never reached the pane")

// promptWithNudge wraps herdr's AgentPrompt/AgentSendKeys/AgentWait/AgentRead
// to submit opts.Text while working around a submission that never actually
// gets typed into the pane (observed in production: the text only appears
// once something else, like an operator's own keypress, nudges herdr's
// terminal-state detection).
//
// It first detects *start*, not completion: it submits with a short grace
// timeout, waiting for either "working" or any of the caller's own Until
// states (a completion so fast it's observed before "working" ever is). If
// neither is observed in that window, it reads the pane's current text and
// compares it against the snapshot taken before the previous attempt: if the
// pane changed at all (the common case — the text landed but wasn't
// submitted), it sends a bare Enter keypress and re-waits, same as before. If
// the pane shows no change whatsoever (the text never reached it in the
// first place, so Enter alone has nothing to submit), it resubmits opts.Text
// in full instead — up to promptMaxRetypes times — rather than nudging blind.
// Retrying either way is capped by promptMaxNudges total attempts, and an
// unchanged pane that's already exhausted promptMaxRetypes retypes gives up
// immediately with errStuckSubmission rather than spending its remaining
// nudge budget on Enter keypresses that can't do anything either.
//
// Once "working" is observed, start/nudge handling is done: this enters a
// completion phase that only waits (never nudges or resubmits) for one of
// opts.Until's caller-requested final states. Both phases are billed
// against a single deadline computed from opts.TimeoutMs at entry — the
// grace window each start/nudge attempt gets shrinks as that budget is
// spent, and completion is handed whatever's left, so a slow completion —
// e.g. "/compact", which can take minutes — gets the remainder of the
// caller's own timeout instead of the fixed grace window, and an
// already-expired deadline is reported as a timeout instead of silently
// becoming an unbounded wait. opts.TimeoutMs <= 0 means no deadline at all:
// start/nudge attempts still use the fixed grace window (bounded), but
// completion then waits with no timeout (unlimited).
func promptWithNudge(
	prompt func(herdr.AgentPromptOptions) (herdr.Agent, error),
	sendKeys func(target string, keys ...string) error,
	wait func(herdr.AgentWaitOptions) (herdr.Agent, error),
	read func(target string, opts herdr.AgentReadOptions) (string, error),
	now func() time.Time,
) func(herdr.AgentPromptOptions) (herdr.Agent, error) {
	return func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if !opts.Wait {
			return prompt(opts)
		}

		startUntil := opts.Until
		if !slices.Contains(startUntil, "working") {
			startUntil = append([]string{"working"}, startUntil...)
		}

		hasDeadline := opts.TimeoutMs > 0
		var deadline time.Time
		if hasDeadline {
			deadline = now().Add(time.Duration(opts.TimeoutMs) * time.Millisecond)
		}

		// budgetMs returns the timeout for the next start/nudge attempt,
		// the fixed grace window capped to what remains of the caller's
		// deadline. ok is false once that deadline has already passed.
		budgetMs := func() (ms int, ok bool) {
			if !hasDeadline {
				return promptNudgeGraceMs, true
			}
			remaining := deadline.Sub(now())
			if remaining <= 0 {
				return 0, false
			}
			if remaining > promptNudgeGraceMs*time.Millisecond {
				return promptNudgeGraceMs, true
			}
			return max(1, int(remaining/time.Millisecond)), true
		}

		graceMs, ok := budgetMs()
		if !ok {
			return herdr.Agent{}, fmt.Errorf("timed out waiting for agent to start: overall deadline exceeded")
		}
		submit := opts
		submit.Until = startUntil
		submit.TimeoutMs = graceMs

		// snapshot is best-effort: a read error is treated the same as "the
		// pane changed" (unchanged stays false below), since there's no
		// evidence to declare it stuck — falling back to the older,
		// safer bare-Enter-nudge behavior rather than resubmitting blind.
		snapshot, _ := read(opts.Target, herdr.AgentReadOptions{Source: "recent-unwrapped"})

		agent, err := prompt(submit)
		retypes := 0
		for attempt := 0; isPollTimeout(err) && attempt < promptMaxNudges; attempt++ {
			graceMs, ok := budgetMs()
			if !ok {
				break
			}

			after, readErr := read(opts.Target, herdr.AgentReadOptions{Source: "recent-unwrapped"})
			unchanged := readErr == nil && after == snapshot
			snapshot = after

			if unchanged {
				if retypes >= promptMaxRetypes {
					return herdr.Agent{}, fmt.Errorf("%w after %d retries: %w", errStuckSubmission, retypes, err)
				}
				retypes++
				resubmit := opts
				resubmit.Until = startUntil
				resubmit.TimeoutMs = graceMs
				agent, err = prompt(resubmit)
				continue
			}

			if nudgeErr := sendKeys(opts.Target, "enter"); nudgeErr != nil {
				return herdr.Agent{}, fmt.Errorf("nudging stuck submission: %w", nudgeErr)
			}
			agent, err = wait(herdr.AgentWaitOptions{
				Target:    opts.Target,
				Until:     startUntil,
				TimeoutMs: graceMs,
			})
		}
		if err != nil {
			return agent, err
		}

		if slices.Contains(opts.Until, agent.AgentStatus) {
			return agent, nil
		}

		completionTimeoutMs := 0
		if hasDeadline {
			remaining := deadline.Sub(now())
			if remaining <= 0 {
				return agent, fmt.Errorf("timed out waiting for completion: overall deadline exceeded")
			}
			completionTimeoutMs = max(1, int(remaining/time.Millisecond))
		}

		return wait(herdr.AgentWaitOptions{
			Target:    opts.Target,
			Until:     opts.Until,
			TimeoutMs: completionTimeoutMs,
		})
	}
}

// worktreeDir implements Deps.WorktreeDir against the real git package.
func worktreeDir(repoDir string) (string, error) {
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return "", err
	}
	return repo.LinkedWorktreeDir(), nil
}

// addWorktree implements Deps.AddWorktree against the real git package. When
// branch already exists — a resumed iteration reattaching to the branch a
// prior park left intact — it attaches the worktree to that branch instead
// of trying (and failing) to create it fresh, so a resume doesn't collide
// with its own pre-park iteration branch.
func addWorktree(repoDir, path, branch, base string) error {
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		// Already checked out, e.g. this Run is reconciling after a crash and
		// a prior invocation already created it.
		return nil
	}
	if git.IsLocalBranch(repo.Root, branch) {
		return git.AddWorktreeOnBranch(*repo, branch, path)
	}
	return git.AddWorktree(*repo, branch, path, base)
}

// removeWorktree implements Deps.RemoveWorktree against the real git package.
func removeWorktree(repoDir, path string, force bool) error {
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return err
	}
	return git.RemoveWorktree(*repo, path, force)
}

// deleteBranch implements Deps.DeleteBranch against the real git package.
func deleteBranch(repoDir, branch string) error {
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return err
	}
	return git.DeleteLocalBranch(*repo, branch, true)
}

// worktreeExists implements Deps.WorktreeExists: whether an iteration
// worktree directory still exists at path.
func worktreeExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// depsMarkers maps a package-manager marker file, checked in this order at
// an iteration worktree's root, to its non-interactive install/sync command.
// go.mod is deliberately not listed: go build/go test populate the module
// cache lazily on first use, so a separate `go mod download` step ahead of
// the agent's own turn buys nothing.
var depsMarkers = []struct {
	marker  string
	command []string
}{
	{"package-lock.json", []string{"npm", "ci"}},
	{"pnpm-lock.yaml", []string{"pnpm", "install", "--frozen-lockfile"}},
	{"yarn.lock", []string{"yarn", "install", "--frozen-lockfile"}},
	{"poetry.lock", []string{"poetry", "install"}},
	{"uv.lock", []string{"uv", "sync"}},
}

// InstallDependencies implements Deps.InstallDeps: it detects path's package
// manager from depsMarkers and runs its install/sync command with path as
// its working directory. No marker matching is a silent no-op (command ""),
// since not every worktree is a package-managed project.
func InstallDependencies(path string) (command string, err error) {
	return installDependenciesWith(path, "")
}

// installDependenciesWith is InstallDependencies with an explicit PATH
// string in place of exec.Command's implicit process-PATH lookup — the seam
// Deps.InstallDeps uses when a per-run PATH override is set. pathEnv == ""
// keeps the real process-PATH lookup exec.Command performs on its own.
func installDependenciesWith(path, pathEnv string) (command string, err error) {
	for _, dm := range depsMarkers {
		if _, statErr := os.Stat(filepath.Join(path, dm.marker)); statErr != nil {
			continue
		}
		command = strings.Join(dm.command, " ")
		name := dm.command[0]
		if pathEnv != "" {
			resolved, lookErr := lookPathIn(name, pathEnv)
			if lookErr != nil {
				return command, fmt.Errorf("running %q in %s: %w", command, path, lookErr)
			}
			name = resolved
		}
		cmd := exec.Command(name, dm.command[1:]...)
		cmd.Dir = path
		out, runErr := cmd.CombinedOutput()
		if runErr != nil {
			return command, fmt.Errorf("running %q in %s: %w\n%s", command, path, runErr, out)
		}
		return command, nil
	}
	return "", nil
}
