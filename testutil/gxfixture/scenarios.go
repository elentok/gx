package main

import (
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/limitseed"
	"github.com/elentok/gx/testutil/gxfixture/spec"
	"github.com/elentok/gx/tickets"
)

const project = limitseed.Project

// oldShapeStatus is a pause status of the old shape: the strict ticket scan
// rejects it until the migration rewrites it.
const oldShapeStatus = "needs-answer"

// The scenario data model lives in the spec package; the aliases keep this
// package's code short.
type (
	ticket         = spec.Ticket
	epic           = spec.Epic
	scenario       = spec.Scenario
	recommendation = spec.Recommendation
	sessionSpec    = spec.SessionSpec
	commandStep    = spec.CommandStep
	restartSpec    = spec.RestartSpec
	webMode        = spec.WebMode
)

const (
	webSignedIn     = spec.WebSignedIn
	webSetupPending = spec.WebSetupPending
	webSignedOut    = spec.WebSignedOut
)

var scenarios = map[string]scenario{
	// empty: a project with no epics, so the Epics page has no cards.
	"empty": {Projects: []string{project}},
	// web-setup: the web setup is pending and the page is not signed in.
	"web-setup": {Projects: []string{project}, Web: webSetupPending},
	// web-login: the setup is done and the page is not signed in.
	"web-login": {Projects: []string{project}, Web: webSignedOut},
	// web-logins: signed in, with three more web logins, one idle for 70 days.
	"web-logins": {Projects: []string{project}, ExtraLogins: []time.Duration{time.Hour, 3 * 24 * time.Hour, 70 * 24 * time.Hour}},
	// limits: a project with no epics and a seeded limits state, shaped by --shape.
	"limits": {Projects: []string{project}, Limits: true},
	// limit-burn: seeded limit history (--shape, see limitburn.go) and one Codex
	// ticket held running. It writes no usage sample of its own: a flow appends a
	// token_count line to the rollout under Output.CodexSessionsDir to make the
	// usage scan save a new Codex sample.
	"limit-burn": {
		Codex:       true,
		Epics:       []epic{{Project: project, Name: "limit-burn-epic", Tickets: []ticket{{ID: "01", Slug: "running", Opts: servertest.TicketOpts{}}}}},
		Queue:       []string{project + ":limit-burn-epic"},
		QueueAgents: map[string]string{project + ":limit-burn-epic": "codex"},
		Hold:        true,
		LimitBurn:   true,
	},
	// ready: an epic that is not queued yet.
	"ready": {
		Epics: []epic{{Project: project, Name: "ready-epic", Tickets: []ticket{
			{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}},
			{ID: "02", Slug: "second", Opts: servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
	},
	// disk-low: an unqueued epic whose project cleans through the disk control files.
	"disk-low": {
		Epics: []epic{{Project: project, Name: "disk-epic", Tickets: []ticket{
			{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}},
			{ID: "02", Slug: "second", Opts: servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
		DiskLow: true,
	},
	// unknown-skill: a queued epic whose agent rejects every skill prompt as an
	// unknown command, so the ticket pauses as agent_prompt_stalled.
	"unknown-skill": {
		Epics:        []epic{{Project: project, Name: "unknown-epic", Tickets: []ticket{{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}}}}},
		Queue:        []string{project + ":unknown-epic"},
		UnknownSkill: true,
	},
	// chat-capture: a server whose Slack and Telegram posts land in the `chat_capture` file
	// (see chat.go); the base of the notification checks. CLI Telegram sends do not reach it.
	"chat-capture":          {ChatCapture: true},
	"rejected-first-prompt": spec.RejectedFirstPrompt,
	"stall-vim":             spec.StallVim,
	"stall-leaves-i":        spec.StallLeavesI,
	"late-registry":         spec.LateRegistry,
	"no-user-record":        spec.NoUserRecord,
	"keep-worktree":         spec.KeepWorktree,
	"keep-agent":            spec.KeepAgent,
	"keep-cleanup":          spec.KeepCleanup,
	"cache-gate":            cacheGate,
	"cache-gate-custom":     cacheGateCustom,
	"launch-held":           launchHeld,
	"launch-fail":           launchFail,
	"epic-at-cap":           epicAtCap,
	// restart-agents: a running agent on a queued epic, and a restart binary that passes its checks.
	"restart-agents": {
		Epics: []epic{{Project: project, Name: "restart-epic", Tickets: []ticket{
			{ID: "01", Slug: "held", Opts: servertest.TicketOpts{}},
			{ID: "02", Slug: "second", Opts: servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
		Queue:   []string{project + ":restart-epic"},
		Hold:    true,
		Restart: &restartSpec{StartsUp: true},
	},
	// restart-projects: two projects, "broken" failing its migration dry run and "demo" passing.
	"restart-projects": {
		Epics: []epic{
			{Project: project, Name: "demo-epic", Tickets: []ticket{{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}}}},
			{Project: "broken", Name: "broken-epic", Tickets: []ticket{{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}}}},
		},
		OwnRepos: []string{"broken"},
		Restart:  &restartSpec{Failing: map[string]string{"broken": "fixture migration failed"}, Pending: map[string]int{"broken": 2}},
	},
	// migrate-pending: a project with no applied list and a ticket in the old shape.
	"migrate-pending": {
		Epics: []epic{{Project: project, Name: "demo-epic", Tickets: []ticket{
			{ID: "01", Slug: "broken-shape", Opts: servertest.TicketOpts{Status: oldShapeStatus}},
		}}},
		Migrate: true,
	},
	// migrate-held: migrate-pending with the migration held open. demo also holds an
	// old-shape ticket the strict index scan rejects, and a ready queued ticket.
	// stuck-epic (every ticket paused) is deadlocked once it is claimed, so it
	// needs a recovery, and merge-epic's last ticket merges the epic when it
	// finishes. Both are queued; the workers are held so a started agent stays
	// running until a flow releases it.
	"migrate-held": {
		Epics: []epic{
			{Project: project, Name: "ready-epic", Tickets: []ticket{{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}}}},
			{Project: project, Name: "old-epic", Tickets: []ticket{{ID: "01", Slug: "broken-shape", Opts: servertest.TicketOpts{Status: oldShapeStatus}}}},
			{Project: project, Name: "stuck-epic", Tickets: []ticket{{ID: "01", Slug: "paused", Opts: servertest.TicketOpts{PauseReason: "iteration-error"}}}},
			{Project: project, Name: "merge-epic", Tickets: []ticket{
				{ID: "01", Slug: "merged", Opts: servertest.TicketOpts{Status: "done"}},
				{ID: "02", Slug: "last", Opts: servertest.TicketOpts{BlockedBy: []string{"01"}}},
			}},
		},
		// A queue add of the whole stuck-epic is refused (no open ticket), so its ticket is queued.
		Queue:       []string{project + ":ready-epic", project + ":stuck-epic/01", project + ":merge-epic"},
		Hold:        true,
		Migrate:     true,
		MigrateHold: true,
	},
	// migrate-broken: project broken fails its migration (a ticket the schema rejects), demo is fine.
	"migrate-broken": {
		Epics: []epic{
			{Project: project, Name: "demo-epic", Tickets: []ticket{{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}}}},
			{Project: "broken", Name: "broken-epic", Tickets: []ticket{
				{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}},
				{ID: "90", Slug: "bad", Opts: servertest.TicketOpts{Status: "bogus"}},
			}},
		},
		OwnRepos: []string{"broken"},
		Queue:    []string{project + ":demo-epic", "broken:broken-epic"},
		Migrate:  true,
	},
	// restart-launchd: the restart binary sits at a launchd-style path, not the server's own.
	"restart-launchd": {
		Epics:   []epic{{Project: project, Name: "restart-epic", Tickets: []ticket{{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}}}}},
		Restart: &restartSpec{Path: spec.RestartAtLaunchd},
	},
	// restart-missing: the restart binary's path has no file behind it.
	"restart-missing": {
		Epics:   []epic{{Project: project, Name: "restart-epic", Tickets: []ticket{{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}}}}},
		Restart: &restartSpec{Path: spec.RestartMissing},
	},
	// health: disk-low plus one fake agent held running on a queued epic, so the
	// busy state can pause the queue and a process table has an agent to tie to.
	"health": {
		Epics: []epic{{Project: project, Name: "health-epic", Tickets: []ticket{
			{ID: "01", Slug: "held", Opts: servertest.TicketOpts{}},
			{ID: "02", Slug: "second", Opts: servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
		Queue:      []string{project + ":health-epic"},
		Hold:       true,
		DiskLow:    true,
		AgentProcs: true,
	},
	// spend: a seeded budget ledger (--shape) and one fake agent held running, so a
	// flow can add spend by appending a usage line to the agent's transcript.
	"spend": {
		Epics: []epic{{Project: project, Name: "spend-epic", Tickets: []ticket{
			{ID: "01", Slug: "held", Opts: servertest.TicketOpts{}},
		}}},
		Queue: []string{project + ":spend-epic"},
		Hold:  true,
		Spend: true,
	},
	// check-lock: one fake agent held running, with a live waiting state for the
	// check lock that a flow removes to end the wait.
	"check-lock": {
		Epics: []epic{{Project: project, Name: "lock-epic", Tickets: []ticket{
			{ID: "01", Slug: "held", Opts: servertest.TicketOpts{}},
		}}},
		Queue:        []string{project + ":lock-epic"},
		Hold:         true,
		CheckWaiting: true,
	},
	// mixed: tickets in several states, and a worker that asks and fails.
	"mixed": {
		Epics: []epic{{Project: project, Name: "mixed-epic", Tickets: []ticket{
			{ID: "01", Slug: "merged", Opts: servertest.TicketOpts{Status: "done"}},
			{ID: "02", Slug: "open", Opts: servertest.TicketOpts{}},
			{ID: "03", Slug: "blocked", Opts: servertest.TicketOpts{BlockedBy: []string{"02"}}},
			{ID: "04", Slug: "drafted", Opts: servertest.TicketOpts{Status: "draft"}},
			{ID: "05", Slug: "dropped", Opts: servertest.TicketOpts{Status: "cancelled"}},
			{ID: "06", Slug: "asks", Opts: servertest.TicketOpts{}},
			{ID: "07", Slug: "fails", Opts: servertest.TicketOpts{}},
		}}},
		Behaviour: map[string]string{
			project + ":mixed-epic/06": agentfake.BehaviourAsk,
			project + ":mixed-epic/07": agentfake.BehaviourFail,
		},
	},
	// long-running: one ticket carries the long-ticket mark, one does not.
	"long-running": {
		Epics: []epic{{Project: project, Name: "long-epic", Tickets: []ticket{
			{ID: "01", Slug: "marked", Opts: servertest.TicketOpts{LongTicketTurns: 120}},
			{ID: "02", Slug: "plain", Opts: servertest.TicketOpts{}},
		}}},
	},
	// finishing: queued, and its last ticket can finish and merge.
	"finishing": {
		Epics: []epic{{Project: project, Name: "finishing-epic", Tickets: []ticket{
			{ID: "01", Slug: "merged", Opts: servertest.TicketOpts{Status: "done"}},
			{ID: "02", Slug: "last", Opts: servertest.TicketOpts{BlockedBy: []string{"01"}}},
		}}},
		Queue: []string{project + ":finishing-epic"},
	},
	// refused-ask: queued, and its only ticket commits, is refused by the finish
	// guard (no `gx check` run) and then asks.
	"refused-ask": {
		Epics: []epic{{Project: project, Name: "refused-epic", Tickets: []ticket{
			{ID: "01", Slug: "refused", Opts: servertest.TicketOpts{}},
		}}},
		Queue:     []string{project + ":refused-epic"},
		Behaviour: map[string]string{project + ":refused-epic/01": agentfake.BehaviourRefuseAsk},
	},
	// blocked-menu: queued, and its only ticket's worker blocks on a question
	// menu, which Approve cannot answer, so only Retry lifts the pause.
	"blocked-menu": {
		Epics: []epic{{Project: project, Name: "menu-epic", Tickets: []ticket{
			{ID: "01", Slug: "menu", Opts: servertest.TicketOpts{}},
		}}},
		Queue:     []string{project + ":menu-epic"},
		Behaviour: map[string]string{project + ":menu-epic/01": agentfake.BehaviourQuestionMenu},
	},
	// rate-limited: both epics are queued. The first epic's worker hits a Claude
	// rate limit, so the hold keeps the second epic's ticket from claiming.
	"rate-limited": {
		Epics: []epic{
			{Project: project, Name: "limited-epic", Tickets: []ticket{{ID: "01", Slug: "limited", Opts: servertest.TicketOpts{}}}},
			{Project: project, Name: "waiting-epic", Tickets: []ticket{{ID: "01", Slug: "waiting", Opts: servertest.TicketOpts{}}}},
		},
		// Keep the waiting ticket queued until the first worker reports its hold.
		ProjectExtra: map[string]string{project: `,"max-agents":1`},
		Queue:        []string{project + ":limited-epic", project + ":waiting-epic"},
		Behaviour:    map[string]string{project + ":limited-epic/01": agentfake.BehaviourRateLimit},

		RateLimitFromUsage: true,
	},
	// usage-gated: the first epic's worker reports the weekly window at 99% and
	// holds, so the usage gate keeps the second epic's ticket from claiming.
	"usage-gated": {
		Epics: []epic{
			{Project: project, Name: "busy-epic", Tickets: []ticket{{ID: "01", Slug: "busy", Opts: servertest.TicketOpts{}}}},
			{Project: project, Name: "gated-epic", Tickets: []ticket{{ID: "01", Slug: "gated", Opts: servertest.TicketOpts{}}}},
		},
		Queue:           []string{project + ":busy-epic"},
		QueueAfterUsage: []string{project + ":gated-epic"},
		Hold:            true,
		Usage:           true,
	},
	// paused: one ticket per recovery path. Every paused ticket opens a pause
	// round. The approve epic is queued, so its worker really blocks on a
	// permission prompt in the pty runner; the other epic is not, so nothing
	// launches under the paused tickets.
	"paused": {
		Epics: []epic{
			{Project: project, Name: "paused-epic", Tickets: []ticket{
				{ID: "01", Slug: "blocker-a", Opts: servertest.TicketOpts{}},
				{ID: "02", Slug: "blocker-b", Opts: servertest.TicketOpts{}},
				{ID: "03", Slug: "answer", Opts: servertest.TicketOpts{PauseReason: "self-reported"}},
				{ID: "04", Slug: "choose-base", Opts: servertest.TicketOpts{PauseReason: "ambiguous-base", BlockedBy: []string{"01", "02"}}},
				{ID: "05", Slug: "resolve-conflict", Opts: servertest.TicketOpts{PauseReason: "merge-conflict"}},
				{ID: "06", Slug: "retry", Opts: servertest.TicketOpts{PauseReason: "zero-commit"}},
				{ID: "07", Slug: "repair", Opts: servertest.TicketOpts{PauseReason: "iteration-error"}},
				{ID: "08", Slug: "manual", Opts: servertest.TicketOpts{PauseReason: "manual"}},
				{ID: "09", Slug: "recommended", Opts: servertest.TicketOpts{PauseReason: "iteration-error"}},
			}},
			{Project: project, Name: "approve-epic", Tickets: []ticket{
				{ID: "01", Slug: "approve", Opts: servertest.TicketOpts{}},
			}},
		},
		Behaviour: map[string]string{project + ":approve-epic/01": agentfake.BehaviourPermission},
		Queue:     []string{project + ":approve-epic"},
		Recommend: map[string]recommendation{
			project + ":paused-epic/09": {Verb: "retry", Diagnosis: "The run hit a transient error; retrying should work."},
		},
		// A merge-conflict pause keeps its iteration branch and the epic's feature
		// worktree, so Merge again has something to merge into.
		GitSetup: func(git spec.Git, repo string) error {
			if err := git(repo, "worktree", "add", "-b", "paused-epic", filepath.Join(repo, ".worktrees", "paused-epic")); err != nil {
				return err
			}
			return spec.CommitOnBranch(git, repo, "ralph-loop/paused-epic-item-05", "conflict.txt", "branch")
		},
	},
	// model-mismatch: one ticket paused because its session ran on a model other
	// than its tier's. The epic is not queued, so nothing launches under it.
	"model-mismatch": {
		Epics: []epic{
			{Project: project, Name: "mismatch-epic", Tickets: []ticket{
				{ID: "01", Slug: "mismatch", Opts: servertest.TicketOpts{PauseReason: "model-mismatch", ModelRan: "claude-opus-4", ModelExpected: "claude-sonnet-4"}},
			}},
		},
	},
	// design: a map epic at the Resolve tickets step, with open tickets. Its
	// design session replays fakeclaude's scripted chat. Its implementation
	// epic exists, so Progress links to it at Branch and Done.
	"design": spec.DesignBase,
	// design-two-live: design plus two live design sessions on design-epic, for
	// the ⋯ menu's "Live sessions" list. The older one is a plain session made by
	// `gx agents create` and marked a design session by `gx agent link` (with
	// GX_AGENT_SESSION set); it stays idle because its prompt has no /gx-design
	// prefix. The newer one is the scripted design session `gx designs open`
	// starts afterwards, so the page shows it with round one (Q8, Q8b) up. The
	// link comes last: a live design session makes `designs open` return it.
	"design-two-live": spec.DesignTwoLive(),
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
	"grilling-stats": spec.GrillingStats(),
	// design-plain (V3): a round, then a plain-text turn after the answer.
	"design-plain": spec.DesignScript(tickets.StepResolveTickets,
		agentfake.DesignTurnRound, agentfake.DesignTurnPlain),
	// design-branch (V4): the map is at Branch and the agent ends the run.
	"design-branch": spec.DesignScript(tickets.StepBranch, agentfake.DesignTurnRunEnd),
	// design-error (V13): a round, then an error turn before the run-end line.
	"design-error": spec.DesignScript(tickets.StepResolveTickets,
		agentfake.DesignTurnRound, agentfake.DesignTurnError),
	// design-guard (V14): six run-end turns in a row.
	"design-guard": spec.DesignScript(tickets.StepResolveTickets,
		slices.Repeat([]string{agentfake.DesignTurnRunEnd}, 6)...),
	// design-mixed: one asked question and two defaulted ones.
	"design-mixed": spec.DesignScript(tickets.StepResolveTickets, agentfake.DesignTurnMixed),
	// design-defaulted: every question of the round is defaulted.
	"design-defaulted": spec.DesignScript(tickets.StepResolveTickets, agentfake.DesignTurnDefaulted),
	// design-text: one free-text question, so the sheet shows an answer textarea.
	"design-text": spec.DesignScript(tickets.StepResolveTickets, agentfake.DesignTurnText),
	// design-working: a long busy turn, for the Working row.
	"design-working": spec.DesignScript(tickets.StepResolveTickets, agentfake.DesignTurnSlow),
	// design-model-tiers: four design sessions, one per launch tier rule. See
	// spec.DesignModelTiers.
	"design-model-tiers": spec.DesignModelTiers(),
	// charting-handoff: three design sessions launched at the chart step whose
	// map has since moved on. See spec.ChartingHandoff.
	"charting-handoff": spec.ChartingHandoff(),
	// united: one card per Epics page section and per Needs-you reason, across
	// two projects. demo has auto-merge off and a two-agent cap, so with the
	// workers held the permission and running epics fill it and the queued
	// epic and one-off wait. auto has auto-merge on.
	"united": {
		Projects:         []string{"auto"},
		Epics:            spec.UnitedEpics(),
		EpicMeta:         spec.UnitedMeta(),
		ManualMerge:      []string{project},
		ProjectExtra:     map[string]string{project: `,"max-agents":2`},
		Hold:             true,
		RecentlyFinished: 2,
		Behaviour:        map[string]string{project + ":perm-epic/01": agentfake.BehaviourPermission},
		DesignSteps: map[string]string{
			project + ":design-question": string(tickets.StepResolveTickets),
			project + ":design-live":     string(tickets.StepResolveTickets),
			project + ":design-stopped":  string(tickets.StepSpec),
			project + ":design-new":      string(tickets.StepChart),
		},
		// design-question's session ends on a question; design-live's does not.
		DesignByMap: map[string][]string{
			project + ":design-question": {agentfake.DesignTurnRound},
			project + ":design-live":     {agentfake.DesignTurnPlain},
		},
		Queue: []string{
			project + ":perm-epic", project + ":running-epic", project + ":queued-epic",
			project + ":one-offs/02",
		},
		OpenDesigns: []string{project + ":design-question", project + ":design-live"},
	},
	// dirty-trunk: a complete epic whose merge is paused because trunk's checkout
	// holds an uncommitted same.txt that the epic's branch also adds, so
	// restoring the stash after the merge conflicts.
	"dirty-trunk": {
		Epics: []epic{{Project: project, Name: "dirty-epic", Tickets: []ticket{
			{ID: "01", Slug: "built", Opts: servertest.TicketOpts{Status: "done"}},
		}}},
		EpicMeta: map[string]string{project + ":dirty-epic": spec.DirtyTrunkMeta("same.txt")},
		GitSetup: func(git spec.Git, repo string) error {
			if err := spec.CommitOnBranch(git, repo, "dirty-epic", "same.txt", "branch"); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(repo, "same.txt"), []byte("local"), 0o644)
		},
	},
	// checks-missing: two projects with a repo and an epic each, neither with check
	// config. demo's repo has a Makefile test target and a package.json with only
	// a build script; auto has its own repo, so a save on demo leaves auto missing.
	"checks-missing": {
		Epics: []epic{
			{Project: project, Name: "demo-epic", Tickets: []ticket{{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}}}},
			{Project: "auto", Name: "auto-epic", Tickets: []ticket{{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}}}},
		},
		NoCheckConfig: true,
		OwnRepos:      []string{"auto"},
		GitSetup: func(git spec.Git, repo string) error {
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
				if err := git(repo, args...); err != nil {
					return err
				}
			}
			return nil
		},
	},
	// many-done: an epic with five merged tickets closed at different times, and
	// one open ticket.
	"many-done": {
		Epics: []epic{{Project: project, Name: "many-done-epic", Tickets: []ticket{
			{ID: "01", Slug: "merged-a", Opts: spec.DoneAt("2026-01-01T10:00:00Z")},
			{ID: "02", Slug: "merged-b", Opts: spec.DoneAt("2026-01-02T10:00:00Z")},
			{ID: "03", Slug: "merged-c", Opts: spec.DoneAt("2026-01-03T10:00:00Z")},
			{ID: "04", Slug: "merged-d", Opts: spec.DoneAt("2026-01-04T10:00:00Z")},
			{ID: "05", Slug: "merged-e", Opts: spec.DoneAt("2026-01-05T10:00:00Z")},
			{ID: "06", Slug: "open", Opts: servertest.TicketOpts{}},
		}}},
	},
	// agents: eight sessions in demo (4), other (2) and site (2), one per row of
	// the Agents mock, and a worktree in demo. See spec.AgentsScenario.
	"agents": spec.AgentsScenario(),
	// agents-promote: agents plus a Codex session with one message, a held Queue
	// agent and two design sessions, for the Promote to Design flows. See spec.PromoteScenario.
	"agents-promote": spec.PromoteScenario(),
	// agents-images: agents plus saved images in the chat of one Claude and one Codex session.
	"agents-images": agentsImagesScenario(),
	// agents-commands: six lean sessions and two design sessions with fixed
	// command lists from both fakes and an extra skill in the feature worktree.
	// -slow delays the list probes past what a flow waits; -failed fails them.
	"agents-commands":        spec.CommandsScenario("", false),
	"agents-commands-slow":   spec.CommandsScenario(spec.SlowProbeDelay, false),
	"agents-commands-failed": spec.CommandsScenario("", true),
	// agents-blocked: agents plus the command fakes, sessions with command
	// entries, every Blocked state, a Rate limited one, a held Queue agent and
	// two design sessions, for the key and permission flows.
	"agents-blocked": spec.BlockedScenario(),
	// default-harness: the global Default harness is claude; gx overrides to codex,
	// demo and dotfiles inherit. One ticket waits behind a held Claude worker, and
	// the Agents tab holds a Claude, a Codex and a model-switched Claude session.
	// Start it with --fakecodex for codex on PATH. See spec.DefaultHarnessScenario.
	"default-harness": spec.DefaultHarnessScenario(),
	// codex-queue: Default harness claude; two running tickets (Codex, Claude) and
	// three waiting (follows default, pinned to Codex, ran on Claude). See spec.CodexQueueScenario.
	"codex-queue": spec.CodexQueueScenario(),
	// codex-design: Default harness codex; one map whose design session starts on
	// Codex. See spec.CodexDesignScenario.
	"codex-design": spec.CodexDesignScenario(),
	// codex-blocked: two Codex tickets (one per epic) whose workers raise a
	// PermissionRequest hook and stay blocked, so each pauses on the approval card
	// after the dwell. One server serves both decisions: Allow once and Deny.
	"codex-blocked": {
		Codex: true,
		Epics: []epic{
			{Project: project, Name: "codex-blocked-epic", Tickets: []ticket{{ID: "01", Slug: "approve", Opts: servertest.TicketOpts{}}}},
			{Project: project, Name: "codex-denied-epic", Tickets: []ticket{{ID: "01", Slug: "deny", Opts: servertest.TicketOpts{}}}},
		},
		Behaviour: map[string]string{
			project + ":codex-blocked-epic/01": agentfake.BehaviourPermission,
			project + ":codex-denied-epic/01":  agentfake.BehaviourPermission,
		},
		Queue: []string{project + ":codex-blocked-epic", project + ":codex-denied-epic"},
		QueueAgents: map[string]string{
			project + ":codex-blocked-epic": "codex",
			project + ":codex-denied-epic":  "codex",
		},
	},
	// codex-error: one Codex ticket whose first turn ends in a model error with no
	// usage-limit signal, so the ticket pauses after the dwell. The Retry prompt
	// finishes it.
	"codex-error": {
		Codex: true,
		Epics: []epic{
			{Project: project, Name: "codex-error-epic", Tickets: []ticket{{ID: "01", Slug: "fail", Opts: servertest.TicketOpts{}}}},
		},
		Behaviour:   map[string]string{project + ":codex-error-epic/01": agentfake.BehaviourError},
		Queue:       []string{project + ":codex-error-epic"},
		QueueAgents: map[string]string{project + ":codex-error-epic": "codex"},
	},
	// codex-rate-limited: a held Codex ticket whose rollout shows the weekly window at
	// 99%, a Codex ticket queued after the sample is stored (so it waits on the Codex
	// gate), and a held Claude ticket the gate leaves running.
	"codex-rate-limited": {
		Codex: true,
		Epics: []epic{
			{Project: project, Name: "codex-running-epic", Tickets: []ticket{{ID: "01", Slug: "running", Opts: servertest.TicketOpts{}}}},
			{Project: project, Name: "codex-waiting-epic", Tickets: []ticket{{ID: "01", Slug: "waiting", Opts: servertest.TicketOpts{}}}},
			{Project: project, Name: "claude-running-epic", Tickets: []ticket{{ID: "01", Slug: "running", Opts: servertest.TicketOpts{}}}},
		},
		Queue:           []string{project + ":codex-running-epic", project + ":claude-running-epic"},
		QueueAfterUsage: []string{project + ":codex-waiting-epic"},
		QueueAgents: map[string]string{
			project + ":codex-running-epic": "codex", project + ":codex-waiting-epic": "codex",
			project + ":claude-running-epic": "claude",
		},
		// Room for all three, so only the Codex gate keeps the waiting ticket queued.
		ProjectExtra: map[string]string{project: `,"max-agents":3`},
		Hold:         true,
		CodexUsage:   true,
	},
	// queue-agents: one PTY Queue agent and one Design agent. See spec.QueueAgentsScenario.
	"queue-agents": spec.QueueAgentsScenario(),
	// agents-failures: four Claude sessions in demo, one per failure (stalled,
	// auth, rate-limit, blocked). See spec.AgentsFailuresScenario.
	"agents-failures": spec.AgentsFailuresScenario(),
	// edge: a project with no epics, an epic with no tickets, and a ticket
	// blocked by a dependency that is not merged.
	"edge": {
		Projects: []string{"bare"},
		Epics: []epic{
			{Project: project, Name: "empty-epic", Tickets: nil},
			{Project: project, Name: "blocked-epic", Tickets: []ticket{
				{ID: "01", Slug: "unmerged", Opts: servertest.TicketOpts{}},
				{ID: "02", Slug: "waiting", Opts: servertest.TicketOpts{BlockedBy: []string{"01"}}},
			}},
		},
	},
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
