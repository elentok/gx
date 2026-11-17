package spec

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// WorkspaceFork is the workspace the fork scenarios put their sources and drafts in.
const WorkspaceFork = "fork-ws"

// forkDesign is the map of the live Design session fork-source holds.
const forkDesign = project + ":fork-design"

// Briefs of the chat-draft scenario, short and different so a flow can tell
// which draft a new chat's first prompt came from.
const (
	BriefForkClaude   = "Brief of the Claude draft"
	BriefForkCodex    = "Brief of the Codex draft"
	BriefForkWorktree = "Brief of the worktree draft"
)

// forkWorkspaceStep creates fork-ws before the sessions are seeded.
var forkWorkspaceStep = []string{"server", "workspaces", "create", WorkspaceFork}

// ForkSourceScenario is fork-ws holding a plain Claude chat, a plain Codex
// chat and a live Design session: the sources a Fork starts from.
func ForkSourceScenario() scenario {
	return scenario{
		Epics:       []epic{{project, "fork-design", nil}},
		EpicMeta:    map[string]string{forkDesign: "status: open\nkind: map\n"},
		DesignSteps: map[string]string{forkDesign: string(tickets.StepResolveTickets)},
		DesignByMap: map[string][]string{forkDesign: {agentfake.DesignTurnPlain}},
		OpenDesigns: []string{forkDesign},
		Chat:        true,
		Codex:       true,
		ForkHooks:   true,
		Sessions: []sessionSpec{
			{Project: project, Prompt: "hello", Read: true, Workspace: WorkspaceFork},
			{Project: project, Harness: "codex", Prompt: "hello", Read: true, Workspace: WorkspaceFork},
		},
		Steps: [][]string{
			forkWorkspaceStep,
			{"server", "workspaces", "move", forkDesign, WorkspaceFork},
		},
	}
}

// ChatDraftScenario is fork-ws holding three chat drafts to start: Claude,
// Codex, and a Claude one with a new-worktree folder for the Start-failure flow.
func ChatDraftScenario() scenario {
	return scenario{
		Epics:     []epic{{project, "chat-draft-epic", []ticket{{"01", "first", servertest.TicketOpts{}}}}},
		ForkHooks: true,
		Codex:     true,
		ChatDrafts: []ChatDraft{
			{Project: project, Name: "claude-draft", Harness: "claude", Model: StrongestModel, Effort: "max",
				Folder: ops.ChatDraftFolderSource, Brief: BriefForkClaude, Workspace: WorkspaceFork},
			{Project: project, Name: "codex-draft", Harness: "codex", Model: "gpt-5.5-codex", Effort: "high",
				Folder: ops.ChatDraftFolderSource, Brief: BriefForkCodex, Workspace: WorkspaceFork},
			{Project: project, Name: "worktree-draft", Harness: "claude", Model: StrongestModel, Effort: "max",
				Folder: ops.ChatDraftFolderWorktree, Brief: BriefForkWorktree, Workspace: WorkspaceFork},
		},
		Steps: [][]string{forkWorkspaceStep},
	}
}
