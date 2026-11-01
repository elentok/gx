package main

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
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
	list := r.sessionsOf(t)
	if len(list) != 11 {
		t.Fatalf("%d sessions, want the eight of agents plus three", len(list))
	}
	claude, codex, exited := list[8], list[9], list[10]
	if claude.Status.State != server.SessionIdle || codex.Harness != "codex" || codex.Status.State != server.SessionIdle || exited.Status.State != server.SessionExited {
		t.Errorf("new sessions = %s/%s, %s/%s, %s", claude.Harness, claude.Status.State, codex.Harness, codex.Status.State, exited.Status.State)
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

func TestCommandsScenario_ChatsHoldThePillsAndOutput(t *testing.T) {
	t.Parallel()
	r := start(t, "agents-commands")
	list := r.sessionsOf(t)
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
	r.stop(t)
}

func TestCommandsScenario_FailedProbesAreDegraded(t *testing.T) {
	t.Parallel()
	r := start(t, "agents-commands-failed")
	list := r.sessionsOf(t)
	if len(list) != 11 {
		t.Fatalf("%d sessions, want 11", len(list))
	}
	if _, degraded := r.commandNames(t, list[8].ID); !degraded {
		t.Error("claude list is not degraded")
	}
	r.stop(t)
}

func TestCommandsScenario_SlowStartsAndFlagIsChecked(t *testing.T) {
	t.Parallel()
	r := startWith(t, "agents-commands-slow", nil, "--command-list-age", "1s")
	if len(r.sessionsOf(t)) != 11 {
		t.Error("slow scenario lacks the sessions")
	}
	r.stop(t)
	out, err := exec.Command(fixtureBin, "--gx", gxBin, "--fakeclaude", fakeBin, "--command-list-age", "soon", "agents-commands").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "--command-list-age") {
		t.Errorf("bad age: err=%v out=%s", err, out)
	}
}
