package ralphloop

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
)

// Deps are the external side-effecting operations the loop drives: herdr
// socket-API calls and git cherry-picking. DefaultDeps wires them to the
// real herdr/git packages; tests substitute fakes so the orchestration in
// loop.go can run without a live herdr server or git process.
type Deps struct {
	FindOrCreateWorkspace func(label, cwd string) (string, error)
	WorktreeCreate        func(opts herdr.WorktreeCreateOptions) (herdr.Worktree, error)
	WorktreeRemove        func(workspaceID string, force bool) error
	TabCreate             func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error)
	AgentStart            func(opts herdr.AgentStartOptions) (herdr.Agent, error)
	AgentPrompt           func(opts herdr.AgentPromptOptions) (herdr.Agent, error)
	AgentWait             func(opts herdr.AgentWaitOptions) (herdr.Agent, error)
	RevParse              func(dir, ref string) (string, error)
	CommitsAhead          func(dir, fromExclusive, toRef string) (int, error)
	CherryPickRange       func(dir, fromExclusive, toInclusive string) error
	CherryPickInProgress  func(dir string) (bool, error)
}

// DefaultDeps wires Deps to the real herdr and git packages.
func DefaultDeps() Deps {
	return Deps{
		FindOrCreateWorkspace: herdr.FindOrCreateWorkspace,
		WorktreeCreate:        herdr.WorktreeCreate,
		WorktreeRemove:        herdr.WorktreeRemove,
		TabCreate:             herdr.TabCreate,
		AgentStart:            herdr.AgentStart,
		AgentPrompt:           herdr.AgentPrompt,
		AgentWait:             herdr.AgentWait,
		RevParse:              git.RevParse,
		CommitsAhead:          git.CommitsAhead,
		CherryPickRange:       git.CherryPickRange,
		CherryPickInProgress:  git.CherryPickInProgress,
	}
}
