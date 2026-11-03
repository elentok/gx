package main

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/transcript"
)

func (r *running) sessionsOf(t *testing.T) []server.SessionInfo {
	t.Helper()
	var list []server.SessionInfo
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

// leanSessionCount is the sessions of agents-commands: seven and two design sessions.
const leanSessionCount = 9

// commandSessionCount is the sessions of agents-blocked but the Queue agent,
// whose place in the list depends on when the worker starts: the eight of
// agents, three, eight Blocked, three in other states, and two design sessions.
const commandSessionCount = 24

// seededSessions is the list without the Queue agent, in creation order.
func (r *running) seededSessions(t *testing.T) []server.SessionInfo {
	t.Helper()
	return slices.DeleteFunc(r.sessionsOf(t), func(s server.SessionInfo) bool { return s.Queue })
}

func (r *running) commandNames(t *testing.T, args ...string) (names []string, degraded bool) {
	t.Helper()
	var res server.SessionCommandsResponse
	out := r.gx(t, append([]string{"agents", "commands", "--json"}, args...)...)
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("commands output %s: %v", out, err)
	}
	for _, e := range res.Entries {
		names = append(names, e.Name)
	}
	return names, res.Degraded
}

func TestCommandsScenario_ListsSessionsAndFeatureSkill(t *testing.T) {
	t.Parallel()
	r := start(t, "agents-commands")
	list := r.seededSessions(t)
	if len(list) != leanSessionCount {
		t.Fatalf("%d sessions, want %d", len(list), leanSessionCount)
	}
	claude, codex, working, exited := list[0], list[4], list[5], list[6]
	for i := 0; i < 4; i++ {
		if list[i].Harness == "codex" || list[i].Status.State != server.SessionIdle {
			t.Errorf("session %d = %s/%s, want an idle Claude session", i, list[i].Harness, list[i].Status.State)
		}
	}
	if codex.Harness != "codex" || codex.Status.State != server.SessionIdle || working.Status.State != server.SessionWorking || exited.Status.State != server.SessionExited {
		t.Errorf("new sessions = %s/%s, %s, %s", codex.Harness, codex.Status.State, working.Status.State, exited.Status.State)
	}
	if !list[7].Design || !list[8].Design || list[8].Harness != "codex" {
		t.Errorf("design sessions = %+v, %+v", list[7], list[8])
	}

	names, degraded := r.commandNames(t, claude.ID)
	if degraded {
		t.Error("claude list is degraded")
	}
	for _, want := range []string{"cost", "html-skill", "deploy-notes", "acme:lint", "mcp__acme-srv__review_pr"} {
		if !strings.Contains(strings.Join(names, " "), want) {
			t.Errorf("claude list %v misses %s", names, want)
		}
	}

	feature := r.FeatureWorktree
	if feature == "" {
		t.Fatal("no feature_worktree in the fixture JSON")
	}
	withFeature, _ := r.commandNames(t, "--project", "demo", "--dir", feature)
	if !strings.Contains(strings.Join(withFeature, " "), featureSkill) {
		t.Errorf("feature list %v misses %s", withFeature, featureSkill)
	}
	repo := strings.TrimSuffix(feature, "/.worktrees/feature")
	without, _ := r.commandNames(t, "--project", "demo", "--dir", repo)
	if strings.Contains(strings.Join(without, " "), featureSkill) {
		t.Errorf("base list %v holds %s", without, featureSkill)
	}
	r.stop(t)
}

// The agents-blocked scenario seeds two dozen sessions one by one, so the
// read-only TestBlockedScenario_* tests share one fixture. The last test to
// finish stops it.
var blocked struct {
	mu   sync.Mutex
	refs int
	r    *running
}

func sharedBlocked(t *testing.T) *running {
	t.Helper()
	blocked.mu.Lock()
	defer blocked.mu.Unlock()
	blocked.refs++
	var kill func()
	t.Cleanup(func() {
		blocked.mu.Lock()
		defer blocked.mu.Unlock()
		if blocked.refs--; blocked.refs > 0 {
			return
		}
		if r := blocked.r; r != nil {
			blocked.r = nil
			r.stop(t)
		} else if kill != nil {
			kill() // the launch failed after the process started
		}
	})
	if blocked.r == nil {
		blocked.r = launch(t, func(k func()) { kill = k }, "agents-blocked", nil)
	}
	return blocked.r
}

func TestBlockedScenario_BlockedSessionsSettleBlocked(t *testing.T) {
	t.Parallel()
	r := sharedBlocked(t)
	list := r.seededSessions(t)
	if len(list) != commandSessionCount {
		t.Fatalf("%d sessions, want %d", len(list), commandSessionCount)
	}
	for i, want := range []string{"claude", "claude", "claude", "claude", "claude", "codex", "codex", "codex"} {
		si := list[11+i]
		harness := si.Harness
		if harness == "" {
			harness = "claude"
		}
		if harness != want || si.Status.State != server.SessionBlocked || si.Failure == nil || si.Failure.Kind != server.FailureKindBlocked {
			t.Errorf("session %d = %s/%s failure=%+v, want a Blocked %s session", 11+i, harness, si.Status.State, si.Failure, want)
		}
	}
}

func TestBlockedScenario_RemainingStatesExist(t *testing.T) {
	t.Parallel()
	r := sharedBlocked(t)
	list := r.seededSessions(t)
	if len(list) != commandSessionCount {
		t.Fatalf("%d sessions, want %d", len(list), commandSessionCount)
	}
	for i, want := range []struct{ state, label string }{
		{server.SessionBlocked, "Agent hit an error"},
		{server.SessionRateLimited, "Rate limited"},
		{server.SessionWorking, ""},
	} {
		si := list[19+i]
		if si.Status.State != want.state || (want.label != "" && (si.Failure == nil || si.Failure.Label != want.label)) {
			t.Errorf("session %d = %s failure=%+v, want %s %q", 19+i, si.Status.State, si.Failure, want.state, want.label)
		}
	}
	if list[1].Status.State != server.SessionWorking {
		t.Errorf("the first Working session = %s", list[7].Status.State)
	}
	var queue []server.SessionInfo
	if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
		queue = slices.DeleteFunc(r.sessionsOf(t), func(s server.SessionInfo) bool { return !s.Queue })
		return len(queue) == 1
	}) {
		t.Fatalf("queue agents = %+v, want one", queue)
	}
	claudeDesign, codexDesign := list[22], list[23]
	if !claudeDesign.Design || claudeDesign.Harness == "codex" || !claudeDesign.AutoAdvance {
		t.Errorf("Claude design = harness %q design %v auto-advance %v", claudeDesign.Harness, claudeDesign.Design, claudeDesign.AutoAdvance)
	}
	if !codexDesign.Design || codexDesign.Harness != "codex" {
		t.Errorf("Codex design = harness %q design %v", codexDesign.Harness, codexDesign.Design)
	}
}

func TestBlockedScenario_ChatsHoldThePillsAndOutput(t *testing.T) {
	t.Parallel()
	r := sharedBlocked(t)
	list := r.seededSessions(t)
	var chat server.SessionChatResponse
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "chat", list[8].ID, "--json")), &chat); err != nil {
		t.Fatal(err)
	}
	pills := map[string]bool{}
	outputs := 0
	for _, m := range chat.Messages {
		switch m.Kind {
		case transcript.ChatKindCommand:
			pills[m.Text] = true
		case transcript.ChatKindOutput:
			outputs++
			if lines := strings.Count(m.Text, "\n") + 1; lines != 14 {
				t.Errorf("output has %d lines, want 14", lines)
			}
		}
	}
	if outputs != 1 || !pills["/cost"] || !pills["/model"] {
		t.Errorf("chat has outputs=%d pills=%v, want one output and the /cost and /model pills", outputs, pills)
	}
	var codexChat server.SessionChatResponse
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "chat", list[9].ID, "--json")), &codexChat); err != nil {
		t.Fatal(err)
	}
	if len(codexChat.Messages) != 1 || codexChat.Messages[0].Kind != transcript.ChatKindCommand || codexChat.Messages[0].Text != "/status" {
		t.Errorf("codex chat = %+v, want one /status pill", codexChat.Messages)
	}
}

func TestCommandsScenario_FailedProbesAreDegraded(t *testing.T) {
	t.Parallel()
	r := start(t, "agents-commands-failed")
	list := r.seededSessions(t)
	if len(list) != leanSessionCount {
		t.Fatalf("%d sessions, want %d", len(list), leanSessionCount)
	}
	if _, degraded := r.commandNames(t, list[0].ID); !degraded {
		t.Error("claude list is not degraded")
	}
	r.stop(t)
}

func TestCommandsScenario_SlowStartsAndFlagIsChecked(t *testing.T) {
	t.Parallel()
	r := startWith(t, "agents-commands-slow", nil, "--command-list-age", "1s")
	if len(r.seededSessions(t)) != leanSessionCount {
		t.Error("slow scenario lacks the sessions")
	}
	r.stop(t)
	out, err := exec.Command(fixtureBin, "--gx", gxBin, "--fakeclaude", fakeBin, "--command-list-age", "soon", "agents-commands").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "--command-list-age") {
		t.Errorf("bad age: err=%v out=%s", err, out)
	}
}
