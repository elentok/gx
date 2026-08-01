package ralphloop

import (
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/transcript"
)

// Deps are the external side-effecting operations the loop drives: herdr
// socket-API calls, git cherry-picking, and the smart-zone guardrail's
// transcript/pause-signal reads. DefaultDeps wires them to the real
// herdr/git/transcript packages; tests substitute fakes so the
// orchestration in loop.go can run without a live herdr server, git
// process, or real Claude Code transcripts.
type Deps struct {
	FindOrCreateWorkspace func(label, cwd string) (string, error)
	WorktreeCreate        func(opts herdr.WorktreeCreateOptions) (herdr.Worktree, error)
	WorktreeOpen          func(opts herdr.WorktreeOpenOptions) (herdr.Worktree, error)
	WorktreeRemove        func(workspaceID string, force bool) error
	TabCreate             func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error)
	TabList               func(workspaceID string) ([]herdr.Tab, error)
	AgentStart            func(opts herdr.AgentStartOptions) (herdr.Agent, error)
	AgentPrompt           func(opts herdr.AgentPromptOptions) (herdr.Agent, error)
	AgentWait             func(opts herdr.AgentWaitOptions) (herdr.Agent, error)
	AgentSendKeys         func(target string, keys ...string) error
	RevParse              func(dir, ref string) (string, error)
	MergeBase             func(dir, refA, refB string) (string, error)
	CommitsAhead          func(dir, fromExclusive, toRef string) (int, error)
	CherryPickRange       func(dir, fromExclusive, toInclusive string) error
	CherryPickInProgress  func(dir string) (bool, error)

	// ReadOccupancy returns the current context occupancy for the Claude
	// Code session launched in cwd, or ok=false if its transcript has no
	// assistant turn yet.
	ReadOccupancy func(cwd, sessionID string) (occupancy int, ok bool, err error)
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
		WorktreeCreate:        herdr.WorktreeCreate,
		WorktreeOpen:          herdr.WorktreeOpen,
		WorktreeRemove:        herdr.WorktreeRemove,
		TabCreate:             herdr.TabCreate,
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
		ReadPaneRecent:        defaultReadPaneRecent,
		ResumeSignaled:        defaultResumeSignaled,
		Sleep:                 time.Sleep,
	}
}
