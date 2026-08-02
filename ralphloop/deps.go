package ralphloop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	FindOrCreateWorkspace func(label, cwd string) (string, error)
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
	DeleteBranch         func(repoDir, branch string) error
	TabCreate            func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error)
	TabClose             func(tabID string) error
	TabList              func(workspaceID string) ([]herdr.Tab, error)
	AgentStart           func(opts herdr.AgentStartOptions) (herdr.Agent, error)
	AgentPrompt          func(opts herdr.AgentPromptOptions) (herdr.Agent, error)
	AgentWait            func(opts herdr.AgentWaitOptions) (herdr.Agent, error)
	AgentSendKeys        func(target string, keys ...string) error
	RevParse             func(dir, ref string) (string, error)
	MergeBase            func(dir, refA, refB string) (string, error)
	CommitsAhead         func(dir, fromExclusive, toRef string) (int, error)
	CherryPickRange      func(dir, fromExclusive, toInclusive string) error
	CherryPickInProgress func(dir string) (bool, error)
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
	// AppendTrailer amends HEAD's commit message to add a ticket-identifying
	// trailer, stamped onto every landed cherry-pick so classifyDoneTicket can
	// still find it later even if a subsequent rebase-plus-manual-conflict-
	// resolution changes the commit's hash and patch-id both.
	AppendTrailer func(dir, key, value string) error
	// TrailerCommitExists reports whether a commit stamped by AppendTrailer is
	// still reachable from ref, used by startup reconciliation as the final
	// fallback once IsAncestor and PatchesApplied both fail to place a done
	// ticket's landed commit.
	TrailerCommitExists func(dir, ref, key, value string) (bool, error)
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
	// ReadCodexContext returns the latest context-token count for the Codex
	// session launched in cwd, or ok=false until its local session data is
	// complete enough to identify that worktree and session.
	ReadCodexContext func(cwd, sessionID string) (tokens int, ok bool, err error)
	// ReadCodexRateLimit returns an exhausted Codex quota for the session
	// launched in cwd, or ok=false when its session data is incomplete or no
	// quota is exhausted.
	ReadCodexRateLimit func(cwd, sessionID string) (limit codexsession.RateLimit, ok bool, err error)
	// ReadPaneRecent returns pane's recent terminal output, used to detect a
	// Claude usage/session rate-limit message.
	ReadPaneRecent func(pane string) (string, error)
	// ResumeSignaled reports (and atomically consumes) whether a `gx
	// ralph-loop resume` signal is waiting at path.
	ResumeSignaled func(path string) (bool, error)
	// Sleep is how a paused loop waits between ResumeSignaled polls.
	Sleep func(time.Duration)
	// Now returns the current time, injectable so a rate-limit reset
	// deadline can be tested without a real wall-clock wait.
	Now func() time.Time
}

// DefaultDeps wires Deps to the real herdr, git, and transcript packages.
func DefaultDeps() Deps {
	return Deps{
		FindOrCreateWorkspace: herdr.EnsureWorkspace,
		WorktreeDir:           worktreeDir,
		AddWorktree:           addWorktree,
		RemoveWorktree:        removeWorktree,
		DeleteBranch:          deleteBranch,
		TabCreate:             herdr.TabCreate,
		TabClose:              herdr.TabClose,
		TabList:               herdr.TabList,
		AgentStart:            herdr.AgentStart,
		AgentPrompt:           herdr.AgentPrompt,
		AgentWait:             herdr.AgentWait,
		AgentSendKeys:         herdr.AgentSendKeys,
		RevParse:              git.RevParse,
		MergeBase:             git.MergeBase,
		CommitsAhead:          git.CommitsAhead,
		CherryPickRange:       git.CherryPickRange,
		CherryPickInProgress:  git.CherryPickInProgress,
		IsAncestor:            git.IsAncestor,
		PatchesApplied:        git.PatchesApplied,
		AppendTrailer:         git.AppendTrailer,
		TrailerCommitExists:   git.TrailerCommitExists,
		WorktreeExists:        worktreeExists,
		InstallDeps:           InstallDependencies,
		ReadOccupancy:         transcript.LastAssistantOccupancy,
		ReadCodexContext:      codexsession.LastContextTokens,
		ReadCodexRateLimit:    codexsession.LastRateLimit,
		ReadPaneRecent:        defaultReadPaneRecent,
		ResumeSignaled:        defaultResumeSignaled,
		Sleep:                 time.Sleep,
		Now:                   time.Now,
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
