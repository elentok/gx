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
	// starting at base (a ref or commit hash; "" for the repo's HEAD). A
	// no-op if path already exists, so a resumed Run can call it again for a
	// worktree a prior invocation already created.
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
	// ReadCompactions returns how many compaction boundaries the Claude Code
	// session launched in cwd hit, or ok=false if its transcript can't be
	// found yet.
	ReadCompactions func(cwd, sessionID string) (count int, ok bool, err error)
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

	// maxParkPolls caps how many times a parked run polls before it returns
	// normally. Zero means uncapped, which is what production wants: a run
	// with nothing runnable and something a human could clear waits for that
	// person however long it takes. It is unexported so only this package's
	// tests can set it — most of them are about the stalled state a run
	// leaves on disk, not about the park itself, and would otherwise wait
	// forever for a human who never arrives.
	maxParkPolls int
}

// DefaultDeps wires Deps to the real herdr, git, and transcript packages.
func DefaultDeps() Deps {
	return Deps{
		PreflightAgent:        preflightAgent,
		VerifySkill:           verifySkill,
		AgentGet:              herdr.AgentGet,
		VerifyCodexSession:    codexsession.VerifyIdentity,
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
		AgentPrompt:           promptWithNudge(herdr.AgentPrompt, herdr.AgentSendKeys, herdr.AgentWait, time.Now),
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
		InstallDeps:           InstallDependencies,
		ReadOccupancy:         transcript.LastAssistantOccupancy,
		ReadCompactions:       transcript.Compactions,
		ReadCodexContext:      codexsession.LastContextTokens,
		ReadCodexRateLimit:    codexsession.LastRateLimit,
		ReadPaneRecent:        defaultReadPaneRecent,
		Sleep:                 time.Sleep,
		Now:                   time.Now,
	}
}

func preflightAgent(agent AgentKind) error {
	return preflightAgentWith(
		agent,
		exec.LookPath,
		func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		},
	)
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

// verifySkill implements Deps.VerifySkill against the real filesystem.
func verifySkill(agent AgentKind, skill string) error {
	return verifySkillWith(agent, skill, os.UserHomeDir, os.Stat)
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
// and returning the underlying poll-timeout error.
const (
	promptNudgeGraceMs = 4_000
	promptMaxNudges    = 2
)

// promptWithNudge wraps herdr's AgentPrompt/AgentSendKeys/AgentWait to submit
// opts.Text exactly once while working around a submission that never
// actually gets typed into the pane (observed in production: the text only
// appears once something else, like an operator's own keypress, nudges
// herdr's terminal-state detection).
//
// It first detects *start*, not completion: it submits with a short grace
// timeout, waiting for either "working" or any of the caller's own Until
// states (a completion so fast it's observed before "working" ever is). If
// neither is observed in that window, it sends a bare Enter keypress and
// re-waits for the same start states, retrying up to promptMaxNudges times
// before returning the underlying poll-timeout error — the prompt itself is
// never resubmitted, only nudged.
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

		agent, err := prompt(submit)
		for attempt := 0; isPollTimeout(err) && attempt < promptMaxNudges; attempt++ {
			graceMs, ok := budgetMs()
			if !ok {
				break
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

// addWorktree implements Deps.AddWorktree against the real git package.
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
	for _, dm := range depsMarkers {
		if _, statErr := os.Stat(filepath.Join(path, dm.marker)); statErr != nil {
			continue
		}
		command = strings.Join(dm.command, " ")
		cmd := exec.Command(dm.command[0], dm.command[1:]...)
		cmd.Dir = path
		out, runErr := cmd.CombinedOutput()
		if runErr != nil {
			return command, fmt.Errorf("running %q in %s: %w\n%s", command, path, runErr, out)
		}
		return command, nil
	}
	return "", nil
}
