package spec

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// sideProject is the second project of the workspaces scenario.
const sideProject = "side"

// Workspace names the workspaces scenario creates, besides the Default workspace.
const (
	WorkspaceBugs     = "gx bugs"
	WorkspaceResearch = "research"
)

// WorkspacesScenario seeds the workspaces the sidebar and the Epics page group
// by: a live Design and a held Design in "gx bugs", an implementation epic with
// a held worker and a held epic in "research", a chat and the one-off agent in
// the Default workspace, and one Epics-page card per section.
//
// Other holds a project-level Fix agent (a failed migration), a Fix agent on a
// paused One-off, and the Investigate agent of another paused One-off. A Fix
// agent on paused-epic/01 and the Investigate agent of inv-epic nest under their epics.
//
// A second project, "side", has its own repo, a Design (side-design) and a chat, so
// two projects can be told apart (New chat lists both).
//
// Footer indicators: the health warning, the paused and the offline indicator can
// all show on one page, steered by the flow: setHealth (webui/e2e/fixture.ts,
// writes the fake CPU and memory file under HealthDir; any scenario has it),
// `gx server queue pause`, and blockEvents (webui/e2e/fixture.ts) for offline.
func WorkspacesScenario() scenario {
	const (
		liveDesign = project + ":ws-design"
		heldDesign = project + ":held-design"
		implEpic   = project + ":ws-impl-epic"
		heldEpic   = project + ":held-epic"
	)
	open := []ticket{{"01", "first", servertest.TicketOpts{}}}
	return scenario{
		Epics: []epic{
			{project, "ws-design", nil},
			{project, "held-design", nil},
			{project, "ws-impl-epic", open},
			{project, "held-epic", open},
			// One-off agent: a queued one-off ticket whose worker is held.
			{project, "one-offs", []ticket{
				// A one-offs epic runs one ticket at a time: 01 is the Investigate agent
				// of the paused 02, and 04 is the paused one a Fix agent is started on.
				{"01", "one-off", servertest.TicketOpts{Type: "investigate", Parent: "02"}},
				{"02", "one-off-paused", servertest.TicketOpts{PauseReason: "iteration-error"}},
				{"04", "one-off-fixed", servertest.TicketOpts{PauseReason: "iteration-error"}},
			}},
			// An Investigate agent of an implementation epic.
			{project, "inv-epic", []ticket{
				{"01", "broken", servertest.TicketOpts{PauseReason: "iteration-error"}},
				{"02", "investigate", servertest.TicketOpts{Type: "investigate", Parent: "01"}},
			}},
			// A second project (own repo, so New chat lists it) with a Design and a chat.
			{sideProject, "side-design", nil},
			// One card per remaining section.
			{project, "paused-epic", []ticket{
				{"01", "repair", servertest.TicketOpts{PauseReason: "iteration-error"}},
				{"02", "repair-two", servertest.TicketOpts{PauseReason: "iteration-error"}},
			}},
			{project, "ready-epic", open},
			{project, "design-new", nil},
			{project, "finished-a", []ticket{{"01", "built", servertest.TicketOpts{Status: "done"}}}},
			// The sidebar's follow-ups badge: draft and open tickets count: 2 bugs, 2 ideas. The project has no Triage.
			{project, "follow-ups", []ticket{
				{"01", "a-bug", servertest.TicketOpts{Category: "bug"}},
				{"02", "another-bug", servertest.TicketOpts{Category: "bug"}},
				{"03", "an-idea", servertest.TicketOpts{Category: "idea"}},
				{"04", "a-draft", servertest.TicketOpts{Category: "idea", Status: "draft"}},
			}},
		},
		EpicMeta: map[string]string{
			project + ":ws-design":       "status: open\nkind: map\n",
			project + ":held-design":     "status: open\nkind: map\n",
			project + ":design-new":      "status: open\nkind: map\n",
			sideProject + ":side-design": "status: open\nkind: map\n",
			project + ":one-offs":        "status: draft\n",
			project + ":finished-a":      "status: open\ncompleted_at: 2026-01-01T10:00:00Z\nmerged_at: 2026-01-01T11:00:00Z\n",
			project + ":ws-impl-epic":    "status: open\nstarted_at: 2026-01-01T10:00:00Z\n",
		},
		DesignSteps: map[string]string{
			liveDesign:                   string(tickets.StepResolveTickets),
			heldDesign:                   string(tickets.StepResolveTickets),
			project + ":design-new":      string(tickets.StepChart),
			sideProject + ":side-design": string(tickets.StepChart),
		},
		OwnRepos: []string{sideProject},
		// design-new is not open at start: a flow starts it from its page and its first turn asks a round.
		DesignByMap: map[string][]string{
			liveDesign:              {agentfake.DesignTurnPlain},
			project + ":design-new": {agentfake.DesignTurnRound},
		},
		OpenDesigns: []string{liveDesign},
		Hold:        true,
		Chat:        true,
		Limits:      true,
		Queue:       []string{implEpic, project + ":one-offs/01", project + ":inv-epic/02"},

		// The fake agent shows no sign of taking an Investigate prompt, so the
		// default grace would hold those agents' launch for 45 seconds.
		PromptGrace:     "1s",
		FailedMigration: project,
		// paused-epic holds two Fix agents: 01 needs you (Idle), 02 is Working.
		FixAgents:        []string{project + ":one-offs/04", project + ":paused-epic/01", project + ":paused-epic/02"},
		WorkingFixAgents: []string{project + ":paused-epic/02"},
		Sessions: []sessionSpec{
			{Project: project, Prompt: "hello", Read: true},
			{Project: sideProject, Prompt: "hello", Read: true},
		},
		Steps: [][]string{
			{"server", "workspaces", "create", WorkspaceBugs},
			{"server", "workspaces", "create", WorkspaceResearch},
			{"server", "workspaces", "move", liveDesign, WorkspaceBugs},
			{"server", "workspaces", "move", heldDesign, WorkspaceBugs},
			{"server", "workspaces", "move", implEpic, WorkspaceResearch},
			{"server", "workspaces", "move", heldEpic, WorkspaceResearch},
			{"server", "hold", heldDesign},
			{"server", "hold", heldEpic},
		},
	}
}
