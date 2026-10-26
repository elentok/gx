package main

import (
	"maps"
	"net"
	"slices"
	"time"

	"github.com/elentok/gx/conversations"
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
	// designByMap scripts the design turns of one map, by address, over design.
	designByMap map[string][]string
	// queue lists the epics put on the queue once the server is up.
	queue []string
	// epicMeta replaces an epic's ticket.md frontmatter (the lines between the
	// "---" fences), by address, e.g. "status: draft\n".
	epicMeta map[string]string
	// manualMerge lists the projects whose auto-ff-merge is off; the others have it on.
	manualMerge []string
	// projectExtra is raw project.json members (leading comma) added per project.
	projectExtra map[string]string
	// hold keeps every worker running until the hold file is removed, as --hold does.
	hold bool
	// recentlyFinished is the server.recently-finished cap; zero keeps the default.
	recentlyFinished int
	// openDesigns lists the maps whose design session is opened once the server is up.
	openDesigns []string
	// conversations seeds the conversation records, and the time recording began
	// (the "started" marker). It is called with the time of seeding, so the days
	// it picks stay inside the stats range.
	conversations func(now time.Time) (started time.Time, recs []conversations.Record)
}

var scenarios = map[string]scenario{
	// empty: a project with no epics, so the Epics page has no cards.
	"empty": {projects: []string{project}},
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
	// grilling-stats: design plus seeded Grilling counts. The totals below are
	// per `gx server stats` range, days counted back from today; "all" is both
	// projects:
	//
	//	         30 days          7 days
	//	         words questions  words questions
	//	demo       310     7        60     2
	//	other      240     5       200     4
	//	all        550    12       260     6
	//
	// Counted days are 25, 10 and 3 days ago. The 4 days before the 25th have
	// no_data (hatched); day 24 has no rounds, so it is a zero bar. The "started"
	// marker is 12 days ago, so a transcript older than that sits before it.
	// One Design record has no counts, so it is not counted.
	"grilling-stats": grillingStats(),
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
	// design-mixed: one asked question and two defaulted ones.
	"design-mixed": designScript(tickets.StepResolveTickets, agentfake.DesignTurnMixed),
	// design-defaulted: every question of the round is defaulted.
	"design-defaulted": designScript(tickets.StepResolveTickets, agentfake.DesignTurnDefaulted),
	// design-working: a long busy turn, for the Working row.
	"design-working": designScript(tickets.StepResolveTickets, agentfake.DesignTurnSlow),
	// united: one card per Epics page section and per Needs-you reason, across
	// two projects. demo has auto-merge off and a two-agent cap, so with the
	// workers held the permission and running epics fill it and the queued
	// epic and one-off wait. auto has auto-merge on.
	"united": {
		projects:         []string{"auto"},
		epics:            unitedEpics(),
		epicMeta:         unitedMeta(),
		manualMerge:      []string{project},
		projectExtra:     map[string]string{project: `,"max-agents":2`},
		hold:             true,
		recentlyFinished: 2,
		behaviour:        map[string]string{project + ":perm-epic/01": agentfake.BehaviourPermission},
		designSteps: map[string]string{
			project + ":design-question": string(tickets.StepResolveTickets),
			project + ":design-live":     string(tickets.StepResolveTickets),
			project + ":design-stopped":  string(tickets.StepSpec),
			project + ":design-new":      string(tickets.StepChart),
		},
		// design-question's session ends on a question; design-live's does not.
		designByMap: map[string][]string{
			project + ":design-question": {agentfake.DesignTurnRound},
			project + ":design-live":     {agentfake.DesignTurnPlain},
		},
		queue: []string{
			project + ":perm-epic", project + ":running-epic", project + ":queued-epic",
			project + ":one-offs/02",
		},
		openDesigns: []string{project + ":design-question", project + ":design-live"},
	},
	// many-done: an epic with five merged tickets closed at different times, and
	// one open ticket.
	"many-done": {
		epics: []epic{{project, "many-done-epic", []ticket{
			{"01", "merged-a", doneAt("2026-01-01T10:00:00Z")},
			{"02", "merged-b", doneAt("2026-01-02T10:00:00Z")},
			{"03", "merged-c", doneAt("2026-01-03T10:00:00Z")},
			{"04", "merged-d", doneAt("2026-01-04T10:00:00Z")},
			{"05", "merged-e", doneAt("2026-01-05T10:00:00Z")},
			{"06", "open", servertest.TicketOpts{}},
		}}},
	},
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
			{"03", "timed", servertest.TicketOpts{Status: "done", ClaimedAt: "2026-01-01T10:00:00Z", DoneAt: "2026-01-01T10:02:30Z"}},
		}},
		{project, "design-epic-impl", []ticket{
			{"01", "build", servertest.TicketOpts{}},
		}},
	},
	designSteps: map[string]string{project + ":design-epic": string(tickets.StepResolveTickets)},
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

// unitedEpics is the united scenario's epics, grouped by section.
func unitedEpics() []epic {
	open := []ticket{{"01", "first", servertest.TicketOpts{}}}
	done := []ticket{{"01", "built", servertest.TicketOpts{Status: "done"}}}
	return []epic{
		// Needs you.
		{project, "paused-epic", []ticket{{"01", "repair", servertest.TicketOpts{PauseReason: "iteration-error"}}}},
		{project, "question-epic", []ticket{{"01", "answer", servertest.TicketOpts{PauseReason: "self-reported"}}}},
		{project, "perm-epic", open},
		// Its epic blocker is unmerged, so the ticket is blocked and nothing can
		// run: the epic is deadlocked. A cycle would not do, since recovery pauses it.
		{project, "deadlock-epic", open},
		{project, "blocked-merge-epic", done},
		{project, "awaiting-epic", done},
		{project, "design-question", nil},
		{project, "design-stopped", nil},
		// In progress.
		{project, "running-epic", open},
		{project, "queued-epic", open},
		{"auto", "awaiting-auto-epic", done},
		{project, "design-live", nil},
		// Ready to queue.
		{project, "ready-epic", open},
		// Ready to design.
		{project, "follow-ups", []ticket{
			{"01", "a-bug", servertest.TicketOpts{Category: "bug"}},
			{"02", "an-idea", servertest.TicketOpts{Category: "idea"}},
		}},
		{project, "design-new", nil},
		{project, "draft-epic", open},
		// A draft -impl epic: its tickets are still being published.
		{project, "publishing-epic-impl", open},
		{project, "idle-epic", []ticket{{"01", "stuck", servertest.TicketOpts{Status: "claimed"}}}},
		// Finished: more than recentlyFinished, one-offs included.
		{project, "finished-a", done},
		{project, "finished-b", done},
		{"auto", "finished-c", done},
		// One-offs: queued, paused and finished.
		{project, "one-offs", []ticket{
			{"01", "paused", servertest.TicketOpts{PauseReason: "iteration-error"}},
			{"02", "queued", servertest.TicketOpts{}},
			{"03", "done-a", servertest.TicketOpts{Status: "done"}},
			{"04", "done-b", servertest.TicketOpts{Status: "done"}},
			{"05", "done-c", servertest.TicketOpts{Status: "done"}},
		}},
	}
}

// unitedMeta is the epic ticket.md frontmatter the united scenario overrides.
func unitedMeta() map[string]string {
	merged := func(day string) string {
		return "status: open\ncompleted_at: 2026-01-" + day + "T10:00:00Z\nmerged_at: 2026-01-" + day + "T11:00:00Z\n"
	}
	return map[string]string{
		project + ":blocked-merge-epic":   "status: open\nland_block_reason: needs a rebase\n",
		project + ":deadlock-epic":        "status: open\nblocked_by: [\"ready-epic\"]\n",
		project + ":draft-epic":           "status: draft\n",
		project + ":publishing-epic-impl": "status: draft\n",
		project + ":finished-a":           merged("01"),
		project + ":finished-b":           merged("02"),
		"auto:finished-c":                 merged("03"),
		project + ":one-offs":             "status: draft\n",
	}
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
