package main

import (
	"maps"
	"net"
	"slices"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

const project = "demo"

type ticket struct {
	id, slug string
	opts     servertest.TicketOpts
}

type epic struct {
	project, name string
	tickets       []ticket
}

type scenario struct {
	// projects have no epics.
	projects []string
	epics    []epic
	// behaviour is the fake worker's per-ticket behaviour, by address.
	behaviour map[string]string
	// designSteps makes each epic, by address, a map and sets its design step
	// with the CLI verb, so no spec file is needed in trunk.
	designSteps map[string]string
	// design scripts the design agent's turns (agentfake.DesignTurn*), one per
	// prompt in order; none means the fake's default.
	design []string
	// queue lists the epics put on the queue once the server is up.
	queue []string
}

var scenarios = map[string]scenario{
	// ready: an epic that is not queued yet.
	"ready": {
		epics: []epic{{project, "ready-epic", []ticket{
			{"01", "first", servertest.TicketOpts{}},
			{"02", "second", servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
	},
	// mixed: tickets in several states, and a worker that asks and fails.
	"mixed": {
		epics: []epic{{project, "mixed-epic", []ticket{
			{"01", "merged", servertest.TicketOpts{Status: "done"}},
			{"02", "open", servertest.TicketOpts{}},
			{"03", "blocked", servertest.TicketOpts{BlockedBy: []string{"02"}}},
			{"04", "drafted", servertest.TicketOpts{Status: "draft"}},
			{"05", "dropped", servertest.TicketOpts{Status: "cancelled"}},
			{"06", "asks", servertest.TicketOpts{}},
			{"07", "fails", servertest.TicketOpts{}},
		}}},
		behaviour: map[string]string{
			project + ":mixed-epic/06": agentfake.BehaviourAsk,
			project + ":mixed-epic/07": agentfake.BehaviourFail,
		},
	},
	// finishing: queued, and its last ticket can finish and merge.
	"finishing": {
		epics: []epic{{project, "finishing-epic", []ticket{
			{"01", "merged", servertest.TicketOpts{Status: "done"}},
			{"02", "last", servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
		queue: []string{project + ":finishing-epic"},
	},
	// paused: one ticket per recovery path. Every paused ticket opens a pause
	// round. The approve epic is queued, so its worker really blocks on a
	// permission prompt in the pty runner; the other epic is not, so nothing
	// launches under the paused tickets.
	"paused": {
		epics: []epic{
			{project, "paused-epic", []ticket{
				{"01", "blocker-a", servertest.TicketOpts{}},
				{"02", "blocker-b", servertest.TicketOpts{}},
				{"03", "answer", servertest.TicketOpts{PauseReason: "self-reported"}},
				{"04", "choose-base", servertest.TicketOpts{PauseReason: "ambiguous-base", BlockedBy: []string{"01", "02"}}},
				{"05", "resolve-conflict", servertest.TicketOpts{PauseReason: "merge-conflict"}},
				{"06", "retry", servertest.TicketOpts{PauseReason: "zero-commit"}},
				{"07", "repair", servertest.TicketOpts{PauseReason: "iteration-error"}},
			}},
			{project, "approve-epic", []ticket{
				{"01", "approve", servertest.TicketOpts{}},
			}},
		},
		behaviour: map[string]string{project + ":approve-epic/01": agentfake.BehaviourPermission},
		queue:     []string{project + ":approve-epic"},
	},
	// design: a map epic at the Resolve tickets step, with open tickets. Its
	// design session replays fakeclaude's scripted chat. Its implementation
	// epic exists, so Progress links to it at Branch and Done.
	"design": designBase,
	// design-plain (V3): a round, then a plain-text turn after the answer.
	"design-plain": designScript(tickets.StepResolveTickets,
		agentfake.DesignTurnRound, agentfake.DesignTurnPlain),
	// design-branch (V4): the map is at Branch and the agent ends the run.
	"design-branch": designScript(tickets.StepBranch, agentfake.DesignTurnRunEnd),
	// design-error (V13): a round, then an error turn before the run-end line.
	"design-error": designScript(tickets.StepResolveTickets,
		agentfake.DesignTurnRound, agentfake.DesignTurnError),
	// design-guard (V14): six run-end turns in a row.
	"design-guard": designScript(tickets.StepResolveTickets,
		slices.Repeat([]string{agentfake.DesignTurnRunEnd}, 6)...),
	// edge: a project with no epics, an epic with no tickets, and a ticket
	// blocked by a dependency that is not merged.
	"edge": {
		projects: []string{"bare"},
		epics: []epic{
			{project, "empty-epic", nil},
			{project, "blocked-epic", []ticket{
				{"01", "unmerged", servertest.TicketOpts{}},
				{"02", "waiting", servertest.TicketOpts{BlockedBy: []string{"01"}}},
			}},
		},
	},
}

var designBase = scenario{
	epics: []epic{
		{project, "design-epic", []ticket{
			{"01", "first", servertest.TicketOpts{}},
			{"02", "second", servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}},
		{project, "design-epic-impl", []ticket{
			{"01", "build", servertest.TicketOpts{}},
		}},
	},
	designSteps: map[string]string{project + ":design-epic": string(tickets.StepResolveTickets)},
}

// designScript is the design scenario with the map at step and the design
// agent playing turns, one per prompt in order.
func designScript(step tickets.DesignStep, turns ...string) scenario {
	sc := designBase
	sc.designSteps = map[string]string{project + ":design-epic": string(step)}
	sc.design = turns
	return sc
}

func scenarioNames() []string {
	return slices.Sorted(maps.Keys(scenarios))
}

// freePort asks the kernel for an unused loopback port.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
