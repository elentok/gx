package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

func TestFinishingScenario_LastTicketFinishesAndMerges(t *testing.T) {
	t.Parallel()
	r := start(t, "finishing")
	const last = "demo:finishing-epic/02"
	deadline := time.Now().Add(90 * time.Second)
	for r.ticketStatus(t, last) != "done" {
		if status := r.ticketStatus(t, last); status == "needs-repair" || time.Now().After(deadline) {
			t.Logf("history:\n%s", r.gx(t, "server", "tickets", "history", last))
			t.Fatalf("ticket %s is %q, want done", last, status)
		}
		time.Sleep(time.Second)
	}
	r.stop(t)
}

func TestRefusedAskScenario_RefusedFinishThenAskPausesSelfReported(t *testing.T) {
	t.Parallel()
	r := start(t, "refused-ask")
	const addr = "demo:refused-epic/01"
	// Each check runs a gx subprocess, so it polls once a second.
	if !testutil.Poll(60*time.Second, time.Second, func() bool { return r.ticketStatus(t, addr) == "needs-answer" }) {
		t.Logf("history:\n%s", r.gx(t, "server", "tickets", "history", addr))
		t.Fatalf("ticket %s is %q, want needs-answer", addr, r.ticketStatus(t, addr))
	}
	d := r.detail(t, addr)
	if d.PauseReason != "self-reported" || d.Latest == nil {
		t.Errorf("pause_reason = %q, latest = %v; want self-reported with an open round", d.PauseReason, d.Latest)
	}
	r.stop(t)
}

// waitBlockedSession waits for the ticket's agent to sit on a blocked pane. It
// stops there: the 15 s dwell before the pause is covered by the injected-Sleep
// tests in ralphloop and by the Playwright pause and codex flows.
func (r *running) waitBlockedSession(t *testing.T, addr string) {
	t.Helper()
	var list []wire.SessionInfo
	if !testutil.Poll(60*time.Second, time.Second, func() bool {
		list = nil
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
			t.Fatal(err)
		}
		return slices.ContainsFunc(list, func(s wire.SessionInfo) bool {
			return s.Ticket == addr && s.Status.State == wire.SessionBlocked
		})
	}) {
		t.Fatalf("no blocked session on %s; sessions = %+v", addr, list)
	}
}

func TestBlockedMenuScenario_QuestionMenuLeavesTheAgentBlocked(t *testing.T) {
	t.Parallel()
	r := start(t, "blocked-menu")
	r.waitBlockedSession(t, "demo:menu-epic/01")
	r.stop(t)
}

func TestCodexBlockedScenario_PermissionRequestShowsAPendingCard(t *testing.T) {
	t.Parallel()
	r := start(t, "codex-blocked")
	for _, addr := range []string{"demo:codex-blocked-epic/01", "demo:codex-denied-epic/01"} {
		r.waitPermission(t, addr)
		r.waitBlockedSession(t, addr)
	}
	r.stop(t)
}

func TestCodexErrorScenario_ModelErrorLeavesTheAgentBlocked(t *testing.T) {
	t.Parallel()
	r := start(t, "codex-error")
	r.waitBlockedSession(t, "demo:codex-error-epic/01")
	r.stop(t)
}

func TestRateLimitedScenario_StartsAndQueuesBothTickets(t *testing.T) {
	t.Parallel()
	r := start(t, "rate-limited")
	for _, addr := range []string{"demo:limited-epic/01", "demo:waiting-epic/01"} {
		if r.detail(t, addr).Address != addr {
			t.Fatalf("ticket %s missing", addr)
		}
	}
	// The first worker's rate limit holds the second ticket's claim.
	const waiting = "demo:waiting-epic/01"
	if !testutil.Poll(60*time.Second, time.Second, func() bool {
		return strings.Contains(r.gx(t, "server", "tickets", "explain", waiting), "rate-limited")
	}) {
		t.Fatalf("explain %s = %q, want the rate-limit hold", waiting, r.gx(t, "server", "tickets", "explain", waiting))
	}
	r.stop(t)
}

func TestUsageGatedScenario_ServerSeesTheUsageSample(t *testing.T) {
	t.Parallel()
	r := start(t, "usage-gated")
	const gated = "demo:gated-epic/01"
	if !testutil.Poll(60*time.Second, time.Second, func() bool {
		return strings.Contains(r.gx(t, "server", "tickets", "explain", gated), "Claude usage: 99% of the weekly limit")
	}) {
		t.Fatalf("explain %s = %q, want the usage gate", gated, r.gx(t, "server", "tickets", "explain", gated))
	}
	r.stop(t)
}

func TestCodexRateLimitedScenario_ServerSeesTheCodexSampleAndGatesOnlyCodex(t *testing.T) {
	t.Parallel()
	r := start(t, "codex-rate-limited")
	const gated = "demo:codex-waiting-epic/01"
	if !testutil.Poll(60*time.Second, time.Second, func() bool {
		return strings.Contains(r.gx(t, "server", "tickets", "explain", gated), "Codex usage: 99%")
	}) {
		t.Fatalf("explain %s = %q, want the Codex usage gate", gated, r.gx(t, "server", "tickets", "explain", gated))
	}
	if out := r.gx(t, "server", "tickets", "explain", "demo:claude-running-epic/01"); strings.Contains(out, "usage:") {
		t.Errorf("Claude ticket explain = %q, want no usage gate", out)
	}
	r.stop(t)
}

const (
	pausedEpic  = "demo:paused-epic/"
	approveAddr = "demo:approve-epic/01"
)

func TestPausedScenario_OneTicketPerRecoveryPath(t *testing.T) {
	t.Parallel()
	r := start(t, "paused")
	cases := []struct {
		id, reason, action string
	}{
		{"03", "self-reported", wire.ActionAnswer},
		{"04", "ambiguous-base", wire.ActionChooseBase},
		{"05", "merge-conflict", wire.ActionMerge},
		{"06", "zero-commit", wire.ActionRetry},
	}
	for _, c := range cases {
		d := r.detail(t, pausedEpic+c.id)
		if d.PauseReason != c.reason || d.PauseLabel == "" {
			t.Errorf("%s: reason %q label %q, want reason %q", c.id, d.PauseReason, d.PauseLabel, c.reason)
		}
		if !slices.Contains(d.Actions, c.action) {
			t.Errorf("%s: actions = %v, want %q", c.id, d.Actions, c.action)
		}
	}
	if bases := r.detail(t, pausedEpic+"04").Bases; !slices.Equal(bases, []string{"01", "02"}) {
		t.Errorf("choose-base bases = %v, want 01 and 02", bases)
	}

	repair := r.detail(t, pausedEpic+"07")
	if repair.PauseReason != "iteration-error" || repair.Status != "needs-repair" {
		t.Errorf("repair ticket: reason %q status %q, want iteration-error and needs-repair", repair.PauseReason, repair.Status)
	}

	d := r.waitPermission(t, approveAddr)
	if d.Permission.Tool != "Bash command" || d.Permission.Description != agentfake.PermissionCommand || !slices.Contains(d.Actions, wire.ActionApprove) {
		t.Errorf("approve ticket: permission %+v actions %v, want the Bash prompt and approve", d.Permission, d.Actions)
	}
	r.stop(t)
}

// roundTypes is the event type of each pause round in the ticket's log.
func (r *running) roundTypes(t *testing.T, project, epic, id string) []string {
	t.Helper()
	root := filepath.Dir(filepath.Dir(r.StateDir))
	paths, _ := filepath.Glob(filepath.Join(root, "data", "gx", "tickets", project, epic, "issues", id+"-*.md"))
	if len(paths) != 1 {
		t.Fatalf("ticket files for %s = %v", id, paths)
	}
	evs, err := tickets.ReadLog(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, round := range tickets.AllRounds(evs) {
		types = append(types, round.Question.Type)
	}
	return types
}

func TestPausedScenario_EachPauseHasTheRoundOfItsKind(t *testing.T) {
	t.Parallel()
	r := start(t, "paused")
	// A question only where the reply goes to the agent.
	for id, want := range map[string]string{
		"03": tickets.EventPauseQuestion,
		"04": tickets.EventPauseQuestion,
		"05": tickets.EventPauseRepair,
		"06": tickets.EventPauseQuestion,
		"07": tickets.EventPauseRepair,
		"08": tickets.EventPauseRepair,
		"09": tickets.EventPauseRepair,
	} {
		if got := r.roundTypes(t, "demo", "paused-epic", id); !slices.Equal(got, []string{want}) {
			t.Errorf("%s rounds = %v, want [%s]", id, got, want)
		}
	}
	if d := r.detail(t, pausedEpic+"09"); d.Recommendation == nil || d.Recommendation.Verb != wire.ActionRetry {
		t.Errorf("09 recommendation = %+v, want retry", d.Recommendation)
	}
	repo := filepath.Join(filepath.Dir(filepath.Dir(r.StateDir)), "repo")
	if out, err := exec.Command("git", "-C", repo, "rev-parse", "--verify", "ralph-loop/paused-epic-item-05").CombinedOutput(); err != nil {
		t.Errorf("05 has no iteration branch: %v\n%s", err, out)
	}
	r.stop(t)
}

func TestDirtyTrunkScenario_RootPausedOnADirtyCheckout(t *testing.T) {
	t.Parallel()
	r := start(t, "dirty-trunk")
	repo := filepath.Join(filepath.Dir(filepath.Dir(r.StateDir)), "repo")
	status, err := exec.Command("git", "-C", repo, "status", "--porcelain").Output()
	if err != nil || !strings.Contains(string(status), "same.txt") {
		t.Errorf("trunk status = %q, %v; want a dirty same.txt", status, err)
	}
	var found bool
	for _, c := range r.cardsOf(t) {
		if c.Name == "dirty-epic" {
			found = true
			if c.MergeBlockKind != "dirty-trunk" || c.Section != wire.SectionNeedsYou {
				t.Errorf("dirty-epic card = %+v, want a needs-you card blocked on dirty-trunk", c)
			}
		}
	}
	if !found {
		t.Error("no dirty-epic card")
	}
	r.stop(t)
}

func TestPausedScenario_WebRecoveryClearsEachPause(t *testing.T) {
	t.Parallel()
	r := start(t, "paused")
	w := r.web(t)

	// The route each verb's button posts to, as webui/src/ticket/recover.ts has it.
	routes := map[string]string{
		wire.ActionAnswer:     "/v1/tickets/answer",
		wire.ActionResume:     "/v1/tickets/resume",
		wire.ActionChooseBase: "/v1/tickets/resume",
		wire.ActionRetry:      "/v1/tickets/retry",
		wire.ActionMerge:      "/v1/tickets/merge",
	}
	ids := []string{"03", "04", "05", "06", "07", "08", "09"}
	for _, id := range ids {
		d := r.detail(t, pausedEpic+id)
		if len(d.Actions) == 0 {
			t.Fatalf("%s has no actions: %+v", id, d)
		}
		verb := d.Actions[0]
		body := map[string]any{"address": d.Address, "state": d.State}
		switch verb {
		case wire.ActionAnswer:
			body["text"] = "go ahead"
		case wire.ActionChooseBase:
			body["base"] = "01"
		}
		w.post(t, routes[verb], body)
	}
	for _, id := range ids {
		// A merge lands the ticket and a retry relaunches it, so only the pause is checked.
		if d := r.detail(t, pausedEpic+id); d.Status == "needs-answer" || d.Status == "needs-repair" {
			t.Errorf("%s is still %q (reason %q) after its recovery", id, d.Status, d.PauseReason)
		}
	}

	p := r.waitPermission(t, approveAddr).Permission
	w.post(t, "/v1/agents/answer", map[string]any{"address": approveAddr, "decision": "allow", "request_id": p.RequestID})
	if d := r.detail(t, approveAddr); d.Permission != nil {
		t.Errorf("permission after approve = %+v, want it cleared", d.Permission)
	}
	r.stop(t)
}

// checksOf is `gx server checks --json`, by project.
func (r *running) checksOf(t *testing.T) map[string]wire.ProjectChecks {
	t.Helper()
	var st wire.ChecksStatus
	if err := json.Unmarshal([]byte(r.gx(t, "server", "checks", "--json")), &st); err != nil {
		t.Fatal(err)
	}
	out := map[string]wire.ProjectChecks{}
	for _, p := range st.Projects {
		out[p.Project] = p
	}
	return out
}

func kindOf(p wire.ProjectChecks, kind string) wire.KindCheck {
	for _, k := range p.Kinds {
		if k.Kind == kind {
			return k
		}
	}
	return wire.KindCheck{}
}

func TestChecksMissingScenarioReportsBothProjectsMissing(t *testing.T) {
	t.Parallel()
	got := start(t, "checks-missing").checksOf(t)
	if !got["demo"].Missing || !got["auto"].Missing {
		t.Fatalf("want demo and auto missing, got %+v", got)
	}
	test := kindOf(got["demo"], "test")
	if test.Recommended == nil || test.Recommended.Source != "Makefile · test" {
		t.Errorf("demo test recommendation = %+v", test.Recommended)
	}
	if tc := kindOf(got["demo"], "typecheck"); tc.Recommended != nil {
		t.Errorf("demo typecheck should have no candidate, got %+v", tc.Recommended)
	}
}

func TestAgentsScenario_EightSessionsInThreeProjectsAndAWorktree(t *testing.T) {
	t.Parallel()
	r := start(t, "agents")
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	type row struct {
		project, harness, state string
		design, unread          bool
		actionable              string
	}
	var got []row
	for _, s := range list {
		got = append(got, row{s.Project, s.Harness, s.Status.State, s.Design, s.Unread.Unread, s.Actionable.Reason})
	}
	want := []row{
		{"demo", "claude", wire.SessionIdle, false, false, ""},
		{"demo", "claude", wire.SessionWorking, false, false, ""},
		{"demo", "claude", wire.SessionIdle, true, false, "needs a decision"},
		{"demo", "codex", wire.SessionIdle, false, false, ""},
		{"other", "claude", wire.SessionIdle, false, true, ""},
		{"other", "claude", wire.SessionIdle, false, false, "needs a decision"},
		{"site", "claude", wire.SessionIdle, true, false, ""},
		{"site", "claude", wire.SessionExited, false, false, ""},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("sessions = %+v\nwant     %+v", got, want)
	}
	if list[7].Exit != wire.SessionExitCrashed {
		t.Errorf("c2 exit = %q, want crashed", list[7].Exit)
	}
	chatLen := func(i int) int {
		var chat wire.SessionChatResponse
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "chat", list[i].ID, "--json")), &chat); err != nil {
			t.Fatal(err)
		}
		return len(chat.Messages)
	}
	if chatLen(0) == 0 {
		t.Error("a1 has no chat messages")
	}
	if n := chatLen(5); n != 0 {
		t.Errorf("b2 has %d chat messages, want none", n)
	}

	var projects []wire.ProjectWorktrees
	if err := json.Unmarshal([]byte(r.gx(t, "project", "worktrees", "--json")), &projects); err != nil {
		t.Fatal(err)
	}
	for _, p := range projects {
		if p.Project == "demo" && p.Root != "" && len(p.Worktrees) == 1 && strings.HasSuffix(p.Worktrees[0], "feature") {
			return
		}
	}
	t.Errorf("project worktrees = %+v, want demo with its root and one worktree", projects)
}

func TestQueueAgentsScenario_ListsAQueueAgentAndADesignAgent(t *testing.T) {
	t.Parallel()
	r := start(t, "queue-agents")
	var queue, design []wire.SessionInfo
	if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
		var list []wire.SessionInfo
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
			t.Fatal(err)
		}
		queue, design = nil, nil
		for _, s := range list {
			switch {
			case s.Queue:
				queue = append(queue, s)
			case s.Design:
				design = append(design, s)
			}
		}
		return len(queue) == 1 && len(design) == 1
	}) {
		t.Fatalf("queue agents = %+v, design agents = %+v, want one each", queue, design)
	}
}

func TestAgentsFailuresScenario_OneSessionPerFailureKind(t *testing.T) {
	t.Parallel()
	r := start(t, "agents-failures")
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, s := range list {
		if s.Failure == nil {
			t.Fatalf("session %s has no failure: %+v", s.ID, s.Status)
		}
		kinds = append(kinds, s.Failure.Kind)
		if s.Failure.Kind == wire.FailureKindRateLimit && !strings.Contains(s.Failure.Text, "resets at") {
			t.Errorf("rate-limit text = %q, want it to say when the limit resets", s.Failure.Text)
		}
	}
	want := []string{wire.FailureKindStalled, wire.FailureKindAuth, wire.FailureKindRateLimit, wire.FailureKindBlocked}
	if !slices.Equal(kinds, want) {
		t.Fatalf("failure kinds = %v, want %v", kinds, want)
	}
}

func TestOtherScenariosReportNoMissingChecks(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"united", "empty"} {
		for p, c := range start(t, name).checksOf(t) {
			if c.Missing {
				t.Errorf("%s: project %s reports missing checks", name, p)
			}
		}
	}
}

func TestRejectedFirstPromptScenario_TheSessionShowsAnUnknownCommandFailure(t *testing.T) {
	t.Parallel()
	r := start(t, "rejected-first-prompt")
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("sessions = %d, want 1", len(list))
	}
	got := list[0].Actionable
	if !got.GxRaised || !strings.Contains(got.Reason, "Unknown command:") {
		t.Errorf("actionable = %+v, want a gx-raised Unknown command reason", got)
	}
	r.stop(t)
}
