package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
)

func (r *running) epicRow(t *testing.T, epic, ticket string) server.EpicDetailRow {
	t.Helper()
	d, err := apiclient.New(r.Socket).EpicDetail(context.Background(), "demo:"+epic)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range d.Rows {
		if row.Address == "demo:"+epic+"/"+ticket {
			return row
		}
	}
	t.Fatalf("epic %s has no row %s: %+v", epic, ticket, d.Rows)
	return server.EpicDetailRow{}
}

func TestCodexQueueScenario_TwoRunningThreeWaitingWithTheirNotes(t *testing.T) {
	t.Parallel()
	r := start(t, "codex-queue")
	running := map[string]string{"codex-running": "codex", "claude-running": "claude"}
	waiting := map[string]string{
		"follows-default": "follows default",
		"pinned":          "pinned",
		"ran-on-claude":   "keeps Claude (ran before)",
	}
	if !testutil.Poll(90*time.Second, time.Second, func() bool {
		for epic, harness := range running {
			if row := r.epicRow(t, epic, "01"); row.RenderedStatus != "claimed" || row.Harness != harness {
				return false
			}
		}
		return true
	}) {
		t.Fatalf("running rows: codex %+v, claude %+v", r.epicRow(t, "codex-running", "01"), r.epicRow(t, "claude-running", "01"))
	}
	for epic, note := range waiting {
		row := r.epicRow(t, epic, "01")
		if row.RenderedStatus != "open" || row.HarnessNote != note {
			t.Errorf("%s: status %q, note %q; want open, %q", epic, row.RenderedStatus, row.HarnessNote, note)
		}
	}
	var items []struct {
		Address string `json:"address"`
		Agent   string `json:"agent"`
	}
	if err := json.Unmarshal([]byte(r.gx(t, "server", "queue", "items", "--json")), &items); err != nil {
		t.Fatal(err)
	}
	agents := map[string]string{}
	for _, it := range items {
		agents[it.Address] = it.Agent
	}
	want := map[string]string{
		"demo:codex-running": "codex", "demo:claude-running": "claude",
		"demo:follows-default": "", "demo:pinned": "", "demo:ran-on-claude": "",
	}
	for addr, agent := range want {
		if got, ok := agents[addr]; !ok || got != agent {
			t.Errorf("queue item %s agent = %q (queued %v), want %q", addr, got, ok, agent)
		}
	}
	r.stop(t)
}

func TestCodexDesignScenario_SessionOnCodexLinkedToTheMapWithTheScriptedRound(t *testing.T) {
	t.Parallel()
	r := start(t, "codex-design")
	var list []server.SessionInfo
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	var designs []server.SessionInfo
	for _, s := range list {
		if s.Design {
			designs = append(designs, s)
		}
	}
	if len(designs) != 1 || designs[0].Harness != "codex" || designs[0].Ticket != "demo:design-epic" {
		t.Fatalf("design sessions = %+v, want one codex session on demo:design-epic", designs)
	}
	rollouts := filepath.Join(fixtureRoot(r), "home", ".codex", "sessions", "*", "*", "*", "rollout-*.jsonl")
	if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
		paths, _ := filepath.Glob(rollouts)
		for _, p := range paths {
			if data, err := os.ReadFile(p); err == nil && strings.Contains(string(data), "Q8b") {
				return true
			}
		}
		return false
	}) {
		t.Errorf("no rollout under %s holds the scripted round", rollouts)
	}
	r.waitChat(t, designs[0].ID, func(c server.SessionChatResponse) bool { return len(c.Messages) > 0 && c.Round != nil })
	r.stop(t)
}
