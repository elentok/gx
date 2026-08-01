package ralphloop

import (
	"os"
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
	RemoveWorktree       func(repoDir, path string, force bool) error
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

	// ReadOccupancy returns the current context occupancy for the Claude
	// Code session launched in cwd, or ok=false if its transcript has no
	// assistant turn yet.
	ReadOccupancy func(cwd, sessionID string) (occupancy int, ok bool, err error)
	// ReadCodexContext returns the latest context-token count for the Codex
	// session launched in cwd, or ok=false until its local session data is
	// complete enough to identify that worktree and session.
	ReadCodexContext func(cwd, sessionID string) (tokens int, ok bool, err error)
	// ReadPaneRecent returns pane's recent terminal output, used to detect a
	// Claude usage/session rate-limit message.
	ReadPaneRecent func(pane string) (string, error)
	// ResumeSignaled reports (and atomically consumes) whether a `gx
	// ralph-loop resume` signal is waiting at path.
	ResumeSignaled func(path string) (bool, error)
	// Sleep is how a paused loop waits between ResumeSignaled polls.
	Sleep func(time.Duration)
}

// DefaultDeps wires Deps to the real herdr, git, and transcript packages.
func DefaultDeps() Deps {
	return Deps{
		FindOrCreateWorkspace: herdr.FindOrCreateWorkspace,
		WorktreeDir:           worktreeDir,
		AddWorktree:           addWorktree,
		RemoveWorktree:        removeWorktree,
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
		ReadOccupancy:         transcript.LastAssistantOccupancy,
		ReadCodexContext:      codexsession.LastContextTokens,
		ReadPaneRecent:        defaultReadPaneRecent,
		ResumeSignaled:        defaultResumeSignaled,
		Sleep:                 time.Sleep,
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
