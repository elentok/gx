package main

import (
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

var designBase = scenario{
	epics: []epic{
		{project, "design-epic", []ticket{
			{"01", "first", servertest.TicketOpts{}},
			{"02", "second", servertest.TicketOpts{BlockedBy: []string{"01"}}},
			{"03", "timed", servertest.TicketOpts{Status: "done", ClaimedAt: "2026-01-01T10:00:00Z", DoneAt: "2026-01-01T10:02:30Z"}},
		}},
		{project, "design-epic-impl", []ticket{
			{"01", "build", servertest.TicketOpts{}},
		}},
	},
	designSteps: map[string]string{project + ":design-epic": string(tickets.StepResolveTickets)},
	epicMeta: map[string]string{
		project + ":design-epic": "status: open\nkind: map\ndesign_started_at: 2026-01-01T09:00:00Z\n",
		// The held worker never stamps started_at, which a running card's time reads.
		project + ":design-epic-impl": "status: open\ncreated_at: 2026-01-01T09:30:00Z\nstarted_at: 2026-01-01T09:45:00Z\n",
	},
}

func designTwoLive() scenario {
	sc := designBase
	sc.linkedDesigns = []string{project + ":design-epic"}
	sc.openDesigns = []string{project + ":design-epic"}
	return sc
}

// Models the tier scenarios configure: the strongest tier and worker tier of
// the demo project, and a worker model the fake claude refuses to start with.
const (
	strongestModel = "opus"
	workerModel    = "sonnet"
	staleProject   = "stale"
	// plainProject sets no tiers of its own, so its design session runs the empty global one.
	plainProject = "plain"
)

var emptyStrongest = []string{`agents.claude.strongest.model=""`, `agents.claude.strongest.effort=""`}

// tierAgents is project.json's agents key: a strongest and a worker model.
func tierAgents(worker string) string {
	return `,"agents":{"claude":{"model":"` + worker + `","effort":"medium","strongest":{"model":"` + strongestModel + `","effort":"max"}}}`
}

// mapMeta is the ticket.md frontmatter of an open map at step.
func mapMeta(step string) string {
	return "status: open\nkind: map\ndesign_step: " + step + "\n"
}

// designModelTiers opens one design session on each of four maps: at chart
// (strongest), at a later step (worker), at a later step with an unconfirmed
// destination (strongest) and with no design step (strongest).
func designModelTiers() scenario {
	addr := func(name string) string { return project + ":" + name }
	return scenario{
		epics: []epic{
			{project, "tier-chart", nil},
			{project, "tier-later", nil},
			{project, "tier-unconfirmed", nil},
			{project, "tier-no-step", nil},
			{plainProject, "tier-empty", nil},
		},
		// A project cannot blank a tier its global config sets, so the global strongest tier is
		// empty and demo, which sets its own, is the only project that has a model.
		config:       emptyStrongest,
		projectExtra: map[string]string{project: tierAgents(workerModel)},
		epicMeta: map[string]string{
			plainProject + ":tier-empty": mapMeta(string(tickets.StepChart)),
			addr("tier-chart"):           mapMeta(string(tickets.StepChart)),
			addr("tier-later"):           mapMeta(string(tickets.StepResolveTickets)),
			addr("tier-unconfirmed"):     mapMeta(string(tickets.StepResolveTickets)) + "destination_unconfirmed: true\n",
			addr("tier-no-step"):         "status: open\nkind: map\n",
		},
		openDesigns: []string{addr("tier-chart"), addr("tier-later"), addr("tier-unconfirmed"), addr("tier-no-step"), plainProject + ":tier-empty"},
	}
}

// chartingHandoff opens a design session on three maps at the chart step, then
// moves each to the next step, so the session is due the handoff to the worker
// tier. The sessions differ in how they get there: auto-advance on (the agent
// ends its run and the countdown runs), auto-advance off (a person presses
// Continue), and a project whose worker model is stale, so the restart fails.
func chartingHandoff() scenario {
	auto, manual, stale := project+":handoff-auto", project+":handoff-manual", staleProject+":handoff-stale"
	return scenario{
		epics: []epic{
			{project, "handoff-auto", nil},
			{project, "handoff-manual", nil},
			{staleProject, "handoff-stale", nil},
		},
		projectExtra: map[string]string{
			project:      tierAgents(workerModel),
			staleProject: tierAgents(agentfake.StaleModel),
		},
		epicMeta: map[string]string{
			auto:   mapMeta(string(tickets.StepChart)),
			manual: mapMeta(string(tickets.StepChart)),
			stale:  mapMeta(string(tickets.StepChart)),
		},
		designByMap: map[string][]string{
			auto:   {agentfake.DesignTurnRunEnd},
			manual: {agentfake.DesignTurnPlain},
			stale:  {agentfake.DesignTurnPlain},
		},
		openDesigns:    []string{auto, manual, stale},
		autoAdvanceOff: []string{manual, stale},
		advanceSteps: map[string]string{
			auto:   string(tickets.StepResolveTickets),
			manual: string(tickets.StepResolveTickets),
			stale:  string(tickets.StepResolveTickets),
		},
	}
}

// grillingStats is the design scenario with conversation records that carry
// fixed Grilling items; see the scenario map for the totals they sum to.
func grillingStats() scenario {
	sc := designBase
	sc.conversations = func(now time.Time) (time.Time, []conversations.Record) {
		noon := func(daysAgo int) time.Time {
			return time.Date(now.Year(), now.Month(), now.Day()-daysAgo, 12, 0, 0, 0, time.Local)
		}
		rec := func(id, ticket string, items ...conversations.GrillingItem) conversations.Record {
			return conversations.Record{ID: id, Dir: "/tmp/" + id, Harness: "claude", Ticket: ticket, Design: true, Grilling: items}
		}
		item := func(id string, daysAgo, questions, words int) conversations.GrillingItem {
			return conversations.GrillingItem{MessageID: id, At: noon(daysAgo), Questions: questions, Words: words}
		}
		return noon(12), []conversations.Record{
			rec("conv-a", "demo:design-epic/01", item("m1", 25, 2, 100), item("m2", 10, 3, 150)),
			rec("conv-b", "other:plan-epic/01", item("m3", 10, 1, 40), item("m4", 3, 4, 200)),
			rec("conv-c", "demo:design-epic/01", item("m5", 3, 2, 60)),
			rec("conv-uncounted", "demo:design-epic/01"),
			// A plain session: not Design, so its record has no counts.
			{ID: "conv-plain", Dir: "/tmp/conv-plain", Harness: "claude", Ticket: "demo:design-epic/01"},
		}
	}
	return sc
}

// doneAt is a merged ticket claimed an hour before it closed at stamp.
func doneAt(stamp string) servertest.TicketOpts {
	closed, _ := time.Parse(time.RFC3339, stamp)
	return servertest.TicketOpts{Status: "done", ClaimedAt: closed.Add(-time.Hour).Format(time.RFC3339), DoneAt: stamp}
}

// designScript is the design scenario with the map at step and the design
// agent playing turns, one per prompt in order.
func designScript(step tickets.DesignStep, turns ...string) scenario {
	sc := designBase
	sc.designSteps = map[string]string{project + ":design-epic": string(step)}
	sc.design = turns
	return sc
}
