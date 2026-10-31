package main

import (
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
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
	// queueAgents is the agent a queued address runs under (`queue add --agent`);
	// an address missing here is queued with no agent, so it follows the default.
	queueAgents map[string]string
	// ranOnClaude lists tickets that get a real Claude run before the queue loop:
	// the fake asks, the ticket is sent back to waiting, and it keeps its recorded
	// session. Their epics are queued by the queue loop like any other.
	ranOnClaude []string
	// transcripts makes the fake Claude record a ticket prompt in its transcript.
	transcripts bool
	// config is --config style key=value overrides of the global config.
	config []string
	// epicMeta replaces an epic's ticket.md frontmatter (the lines between the
	// "---" fences), by address, e.g. "status: draft\n".
	epicMeta map[string]string
	// manualMerge lists the projects whose auto-ff-merge is off; the others have it on.
	manualMerge []string
	// projectExtra is raw project.json members (leading comma) added per project.
	projectExtra map[string]string
	// hold keeps every worker running until the hold file is removed, as --hold does.
	hold bool
	// agentProcs writes the first fake agent's pids into the process table once it runs.
	agentProcs bool
	// recentlyFinished is the server.recently-finished cap; zero keeps the default.
	recentlyFinished int
	// openDesigns lists the maps whose design session is opened once the server is up.
	openDesigns []string
	// autoAdvanceOff lists the opened maps whose design session gets auto-advance off.
	autoAdvanceOff []string
	// advanceSteps moves each map, by address, to a design step once its design
	// session is open, as the agent does while it charts.
	advanceSteps map[string]string
	// linkedDesigns lists the maps that get an extra plain session, linked to the
	// map as a design session before openDesigns runs, so it is the older run.
	linkedDesigns []string
	// conversations seeds the conversation records, and the time recording began
	// (the "started" marker). It is called with the time of seeding, so the days
	// it picks stay inside the stats range.
	conversations func(now time.Time) (started time.Time, recs []conversations.Record)
	// recommend stores an investigate recommendation on the open round of a
	// paused ticket, by ticket address.
	recommend map[string]recommendation
	// gitSetup shapes the project repo (branches, a dirty checkout) after its first commit.
	gitSetup func(f *fixture, repo string) error
	// noCheckConfig leaves the projects without a checks key in project.json; the
	// other scenarios write an empty one, so the Epics page shows no banner.
	noCheckConfig bool
	// ownRepos lists the projects that get a repo of their own instead of the shared one.
	ownRepos []string
	// queueAfterUsage lists the epics queued once the server has stored a usage
	// sample, so the usage gate is already up when they first try to claim.
	queueAfterUsage []string
	// usage makes the fake workers report the weekly window at 99%.
	usage bool
	// diskLow gives each project a cleanup command that follows the disk's clean file.
	diskLow bool
	// chat turns the fake's chat knob on, so a plain prompt is a chat turn.
	chat bool
	// sessions are created in order once the server is up, as `gx agents create` makes them.
	sessions []sessionSpec
	// codex installs the fake codex on PATH without the --fakecodex flag, for a
	// scenario that runs a Codex session.
	codex bool
	// stallThreshold is how long a Working session's transcript may stay untouched
	// before it counts as stalled; the --stall-threshold flag overrides it.
	stallThreshold string
	// web is how far the fixture takes the web login; the zero value signs in.
	web webMode
	// extraLogins seeds web logins before the server starts, as how long ago each was last used.
	extraLogins []time.Duration
}

// webMode is how far the fixture takes the web setup and sign-in.
type webMode int

const (
	// webSignedIn finishes the setup and keeps the login cookie.
	webSignedIn webMode = iota
	// webSetupPending leaves the setup undone.
	webSetupPending
	// webSignedOut finishes the setup and drops the login cookie.
	webSignedOut
)

type recommendation struct{ verb, diagnosis string }

var scenarios = map[string]scenario{
	// empty: a project with no epics, so the Epics page has no cards.
	"empty": {projects: []string{project}},
	// web-setup: the web setup is pending and the page is not signed in.
	"web-setup": {projects: []string{project}, web: webSetupPending},
	// web-login: the setup is done and the page is not signed in.
	"web-login": {projects: []string{project}, web: webSignedOut},
	// web-logins: signed in, with three more web logins, one idle for 70 days.
	"web-logins": {projects: []string{project}, extraLogins: []time.Duration{time.Hour, 3 * 24 * time.Hour, 70 * 24 * time.Hour}},
	// ready: an epic that is not queued yet.
	"ready": {
		epics: []epic{{project, "ready-epic", []ticket{
			{"01", "first", servertest.TicketOpts{}},
			{"02", "second", servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
	},
	// disk-low: an unqueued epic whose project cleans through the disk control files.
	"disk-low": {
		epics: []epic{{project, "disk-epic", []ticket{
			{"01", "first", servertest.TicketOpts{}},
			{"02", "second", servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
		diskLow: true,
	},
	// health: disk-low plus one fake agent held running on a queued epic, so the
	// busy state can pause the queue and a process table has an agent to tie to.
	"health": {
		epics: []epic{{project, "health-epic", []ticket{
			{"01", "held", servertest.TicketOpts{}},
			{"02", "second", servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
		queue:      []string{project + ":health-epic"},
		hold:       true,
		diskLow:    true,
		agentProcs: true,
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
	// refused-ask: queued, and its only ticket commits, is refused by the finish
	// guard (no `gx check` run) and then asks.
	"refused-ask": {
		epics: []epic{{project, "refused-epic", []ticket{
			{"01", "refused", servertest.TicketOpts{}},
		}}},
		queue:     []string{project + ":refused-epic"},
		behaviour: map[string]string{project + ":refused-epic/01": agentfake.BehaviourRefuseAsk},
	},
	// blocked-menu: queued, and its only ticket's worker blocks on a question
	// menu, which Approve cannot answer, so only Retry lifts the pause.
	"blocked-menu": {
		epics: []epic{{project, "menu-epic", []ticket{
			{"01", "menu", servertest.TicketOpts{}},
		}}},
		queue:     []string{project + ":menu-epic"},
		behaviour: map[string]string{project + ":menu-epic/01": agentfake.BehaviourQuestionMenu},
	},
	// rate-limited: both epics are queued. The first epic's worker hits a Claude
	// rate limit, so the hold keeps the second epic's ticket from claiming.
	"rate-limited": {
		epics: []epic{
			{project, "limited-epic", []ticket{{"01", "limited", servertest.TicketOpts{}}}},
			{project, "waiting-epic", []ticket{{"01", "waiting", servertest.TicketOpts{}}}},
		},
		// Keep the waiting ticket queued until the first worker reports its hold.
		projectExtra: map[string]string{project: `,"max-agents":1`},
		queue:        []string{project + ":limited-epic", project + ":waiting-epic"},
		behaviour:    map[string]string{project + ":limited-epic/01": agentfake.BehaviourRateLimit},
	},
	// usage-gated: the first epic's worker reports the weekly window at 99% and
	// holds, so the usage gate keeps the second epic's ticket from claiming.
	"usage-gated": {
		epics: []epic{
			{project, "busy-epic", []ticket{{"01", "busy", servertest.TicketOpts{}}}},
			{project, "gated-epic", []ticket{{"01", "gated", servertest.TicketOpts{}}}},
		},
		queue:           []string{project + ":busy-epic"},
		queueAfterUsage: []string{project + ":gated-epic"},
		hold:            true,
		usage:           true,
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
				{"08", "manual", servertest.TicketOpts{PauseReason: "manual"}},
				{"09", "recommended", servertest.TicketOpts{PauseReason: "iteration-error"}},
			}},
			{project, "approve-epic", []ticket{
				{"01", "approve", servertest.TicketOpts{}},
			}},
		},
		behaviour: map[string]string{project + ":approve-epic/01": agentfake.BehaviourPermission},
		queue:     []string{project + ":approve-epic"},
		recommend: map[string]recommendation{
			project + ":paused-epic/09": {"retry", "The run hit a transient error; retrying should work."},
		},
		// A merge-conflict pause keeps its iteration branch and the epic's feature
		// worktree, so Merge again has something to merge into.
		gitSetup: func(f *fixture, repo string) error {
			if err := f.git(repo, "worktree", "add", "-b", "paused-epic", filepath.Join(repo, ".worktrees", "paused-epic")); err != nil {
				return err
			}
			return f.commitOnBranch(repo, "ralph-loop/paused-epic-item-05", "conflict.txt", "branch")
		},
	},
	// design: a map epic at the Resolve tickets step, with open tickets. Its
	// design session replays fakeclaude's scripted chat. Its implementation
	// epic exists, so Progress links to it at Branch and Done.
	"design": designBase,
	// design-two-live: design plus two live design sessions on design-epic, for
	// the ⋯ menu's "Live sessions" list. The older one is a plain session made by
	// `gx agents create` and marked a design session by `gx agent link` (with
	// GX_AGENT_SESSION set); it stays idle because its prompt has no /gx-design
	// prefix. The newer one is the scripted design session `gx designs open`
	// starts afterwards, so the page shows it with round one (Q8, Q8b) up. The
	// link comes last: a live design session makes `designs open` return it.
	"design-two-live": designTwoLive(),
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
	// design-model-tiers: four design sessions, one per launch tier rule. See
	// designModelTiers.
	"design-model-tiers": designModelTiers(),
	// charting-handoff: three design sessions launched at the chart step whose
	// map has since moved on. See chartingHandoff.
	"charting-handoff": chartingHandoff(),
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
	// dirty-trunk: a complete epic whose merge is paused because trunk's checkout
	// holds an uncommitted same.txt that the epic's branch also adds, so
	// restoring the stash after the merge conflicts.
	"dirty-trunk": {
		epics: []epic{{project, "dirty-epic", []ticket{
			{"01", "built", servertest.TicketOpts{Status: "done"}},
		}}},
		epicMeta: map[string]string{project + ":dirty-epic": dirtyTrunkMeta("same.txt")},
		gitSetup: func(f *fixture, repo string) error {
			if err := f.commitOnBranch(repo, "dirty-epic", "same.txt", "branch"); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(repo, "same.txt"), []byte("local"), 0o644)
		},
	},
	// checks-missing: two projects with a repo and an epic each, neither with check
	// config. demo's repo has a Makefile test target and a package.json with only
	// a build script; auto has its own repo, so a save on demo leaves auto missing.
	"checks-missing": {
		epics: []epic{
			{project, "demo-epic", []ticket{{"01", "first", servertest.TicketOpts{}}}},
			{"auto", "auto-epic", []ticket{{"01", "first", servertest.TicketOpts{}}}},
		},
		noCheckConfig: true,
		ownRepos:      []string{"auto"},
		gitSetup: func(f *fixture, repo string) error {
			files := map[string]string{
				"Makefile":     "test:\n\tgo test ./...\n",
				"package.json": `{"name":"demo","scripts":{"build":"tsc"}}` + "\n",
			}
			for name, body := range files {
				if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
					return err
				}
			}
			for _, args := range [][]string{{"add", "."}, {"commit", "-m", "build files"}} {
				if err := f.git(repo, args...); err != nil {
					return err
				}
			}
			return nil
		},
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
	// agents: eight sessions in demo (4), other (2) and site (2), one per row of
	// the Agents mock, and a worktree in demo. See agentsScenario.
	"agents": agentsScenario(),
	// default-harness: the global Default harness is claude; gx overrides to codex,
	// demo and dotfiles inherit. One ticket waits behind a held Claude worker, and
	// the Agents tab holds a Claude, a Codex and a model-switched Claude session.
	// Start it with --fakecodex for codex on PATH. See defaultHarnessScenario.
	"default-harness": defaultHarnessScenario(),
	// codex-queue: Default harness claude; two running tickets (Codex, Claude) and
	// three waiting (follows default, pinned to Codex, ran on Claude). See codexQueueScenario.
	"codex-queue": codexQueueScenario(),
	// codex-design: Default harness codex; one map whose design session starts on
	// Codex. See codexDesignScenario.
	"codex-design": codexDesignScenario(),
	// queue-agents: one PTY Queue agent and one Design agent. See queueAgentsScenario.
	"queue-agents": queueAgentsScenario(),
	// agents-failures: four Claude sessions in demo, one per failure (stalled,
	// auth, rate-limit, blocked). See agentsFailuresScenario.
	"agents-failures": agentsFailuresScenario(),
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
)

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
		},
		projectExtra: map[string]string{project: tierAgents(workerModel)},
		epicMeta: map[string]string{
			addr("tier-chart"):       mapMeta(string(tickets.StepChart)),
			addr("tier-later"):       mapMeta(string(tickets.StepResolveTickets)),
			addr("tier-unconfirmed"): mapMeta(string(tickets.StepResolveTickets)) + "destination_unconfirmed: true\n",
			addr("tier-no-step"):     "status: open\nkind: map\n",
		},
		openDesigns: []string{addr("tier-chart"), addr("tier-later"), addr("tier-unconfirmed"), addr("tier-no-step")},
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
		{project, "deadlock-epic", []ticket{{"01", "first", servertest.TicketOpts{ClaimedAt: "2026-01-01T10:00:00Z"}}}},
		{project, "blocked-merge-epic", done},
		{project, "awaiting-epic", done},
		{project, "dirty-trunk-epic", done},
		{project, "design-question", nil},
		{project, "design-stopped", nil},
		// Implementing and Designing.
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
	const (
		oldCreated = "created_at: 2026-01-01T09:00:00Z\n"
		doneStamp  = "completed_at: 2026-01-01T10:00:00Z\n"
	)
	meta := map[string]string{
		project + ":blocked-merge-epic":   "status: open\nland_block_reason: needs a rebase\n" + doneStamp,
		project + ":dirty-trunk-epic":     dirtyTrunkMeta("a.txt", "b.txt", "c.txt", "d.txt", "e.txt", "f.txt", "g.txt") + doneStamp,
		project + ":awaiting-epic":        "status: open\n" + doneStamp,
		project + ":deadlock-epic":        "status: open\nblocked_by: [\"ready-epic\"]\n",
		project + ":draft-epic":           "status: draft\n",
		project + ":publishing-epic-impl": "status: draft\n",
		project + ":finished-a":           merged("01"),
		project + ":finished-b":           merged("02"),
		"auto:finished-c":                 merged("03"),
		project + ":one-offs":             "status: draft\n",
		project + ":running-epic":         "status: open\nstarted_at: 2026-01-01T10:00:00Z\n",
		"auto:awaiting-auto-epic":         "status: open\nstarted_at: 2026-01-01T09:30:00Z\n" + doneStamp,
		// The maps keep their kind, which this override replaces. design-live's
		// stamp comes from its opened session; design-stopped has none, so it
		// gets one here.
		project + ":design-question": "status: open\nkind: map\n",
		project + ":design-live":     "status: open\nkind: map\n",
		project + ":design-stopped":  "status: open\nkind: map\ndesign_started_at: 2026-01-01T09:30:00Z\n",
		project + ":design-new":      "status: open\nkind: map\n",
	}
	// Every epic is created; ready-epic minutes ago, relative to fixture start.
	recent := "created_at: " + time.Now().Add(-5*time.Minute).UTC().Format(time.RFC3339) + "\n"
	for _, e := range unitedEpics() {
		addr := e.project + ":" + e.name
		created := oldCreated
		if addr == project+":ready-epic" {
			created = recent
		}
		if _, ok := meta[addr]; !ok {
			meta[addr] = "status: open\n"
		}
		meta[addr] += created
	}
	return meta
}

// dirtyTrunkMeta is the epic frontmatter of a merge paused on a dirty trunk,
// with the full clashing file list stored as the server stores it.
func dirtyTrunkMeta(files ...string) string {
	reason := fmt.Sprintf("uncommitted changes on main clash with %s; commit or discard them, then merge again", strings.Join(files[:min(len(files), 5)], ", "))
	if extra := len(files) - 5; extra > 0 {
		reason += fmt.Sprintf(" (+%d more)", extra)
	}
	return "status: open\nland_block_kind: dirty-trunk\nland_block_reason: " + strconv.Quote(reason) +
		"\nland_block_files: [" + strings.Join(files, ", ") + "]\n"
}

// commitOnBranch creates branch off main with one committed file and returns to main.
func (f *fixture) commitOnBranch(repo, branch, file, content string) error {
	if err := f.git(repo, "checkout", "-b", branch); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(repo, file), []byte(content), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", file}, {"commit", "-m", branch}, {"checkout", "main"}} {
		if err := f.git(repo, args...); err != nil {
			return err
		}
	}
	return nil
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
