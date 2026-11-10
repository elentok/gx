package spec

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// Workspace the bulk-actions scenario creates, besides the Default workspace.
const WorkspaceBulk = "bulk"

// ArchivedWorkspaceGone is the stored workspace of an archived epic whose workspace no longer exists.
const ArchivedWorkspaceGone = "gone-ws"

// BulkActionsScenario seeds what the bulk actions act on: finished epics (with
// and without trunk evidence), a queued one, a held one, Designs before, at and
// past done (one with an -impl epic), a live Design and Fix session, a Queue
// agent, an Exited agent, and archived epics: with and without archived_at, one
// whose stored workspace is gone, a Design pair, and one whose name an active
// epic also has. Names stay short enough for a 390 px sidebar row.
func BulkActionsScenario() scenario {
	const (
		liveDesign = project + ":bulk-early"
		heldEpic   = project + ":bulk-held"
		queued     = project + ":bulk-queued"
		fixTicket  = project + ":bulk-paused/01"
	)
	open := []ticket{{"01", "first", servertest.TicketOpts{}}}
	done := []ticket{{"01", "built", servertest.TicketOpts{Status: "done"}}}
	return scenario{
		Epics: []epic{
			{project, "bulk-merged", done},
			{project, "bulk-unmerged", done},
			{project, "bulk-queued", open},
			{project, "bulk-held", open},
			{project, "bulk-paused", []ticket{{"01", "repair", servertest.TicketOpts{PauseReason: "iteration-error"}}}},
			{project, "bulk-done", nil},
			{project, "bulk-done-impl", open},
			{project, "bulk-done-solo", nil},
			{project, "bulk-early", nil},
			{project, "bulk-clash", done},
		},
		EpicMeta: map[string]string{
			project + ":bulk-merged":   "status: open\ncompleted_at: 2026-01-01T10:00:00Z\nmerged_at: 2026-01-01T11:00:00Z\n",
			project + ":bulk-unmerged": "status: open\ncompleted_at: 2026-01-02T10:00:00Z\n",
			project + ":bulk-clash":    "status: open\ncompleted_at: 2026-01-03T10:00:00Z\nmerged_at: 2026-01-03T11:00:00Z\n",
		},
		DesignSteps: map[string]string{
			project + ":bulk-done":      string(tickets.StepDone),
			project + ":bulk-done-solo": string(tickets.StepDone),
			liveDesign:                  string(tickets.StepChart),
		},
		Archived: []ArchivedEpic{
			{Project: project, Name: "arch-stamped", Meta: "status: open\narchived_at: 2026-02-01T10:00:00Z\narchived_workspace: " + WorkspaceBulk + "\n", Tickets: done},
			{Project: project, Name: "arch-bare", Meta: "status: open\n", Tickets: done},
			{Project: project, Name: "arch-stale", Meta: "status: open\narchived_at: 2026-02-02T10:00:00Z\narchived_workspace: " + ArchivedWorkspaceGone + "\n", Tickets: done},
			{Project: project, Name: "arch-pair", Meta: "status: open\nkind: map\ndesign_step: done\narchived_at: 2026-02-03T10:00:00Z\n"},
			{Project: project, Name: "arch-pair-impl", Meta: "status: open\narchived_at: 2026-02-03T10:00:00Z\n", Tickets: done},
			// The active bulk-clash has this name too.
			{Project: project, Name: "bulk-clash", Meta: "status: open\narchived_at: 2026-02-04T10:00:00Z\n", Tickets: done},
		},
		DesignByMap: map[string][]string{liveDesign: {agentfake.DesignTurnPlain}},
		OpenDesigns: []string{liveDesign},
		Hold:        true,
		Chat:        true,
		Queue:       []string{queued},
		PromptGrace: "1s",
		FixAgents:   []string{fixTicket},
		Sessions:    []sessionSpec{{Project: project, Prompt: "hello", Crash: true, Read: true}},
		Steps: [][]string{
			{"server", "workspaces", "create", WorkspaceBulk},
			{"server", "workspaces", "move", heldEpic, WorkspaceBulk},
			{"server", "hold", heldEpic},
		},
	}
}
