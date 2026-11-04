package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/transcript"
)

func TestDesignScenario_HasATimedDoneTicket(t *testing.T) {
	t.Parallel()
	r := start(t, "design")
	data := r.issueFiles(t, "demo", "design-epic")["03-timed.md"]
	for _, want := range []string{"status: done\n", "claimed_at: 2026-01-01T10:00:00Z\n", "done_at: 2026-01-01T10:02:30Z\n"} {
		if !strings.Contains(data, want) {
			t.Errorf("timed ticket lacks %q:\n%s", want, data)
		}
	}
	r.stop(t)
}

func TestDesignScenario_StampsTheMapsDesignStart(t *testing.T) {
	t.Parallel()
	r := start(t, "design")
	root := filepath.Dir(filepath.Dir(r.StateDir))
	meta, err := os.ReadFile(filepath.Join(root, "data", "gx", "tickets", "demo", "design-epic", "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(meta), "design_started_at: ") || !strings.Contains(string(meta), "kind: map") {
		t.Errorf("design-epic ticket.md:\n%s", meta)
	}
	r.stop(t)
}

func TestConfigFlag_WritesTheKeyIntoTheFixtureConfig(t *testing.T) {
	t.Parallel()
	r := startWith(t, "empty", nil, "--config", "server.remote-control=true", "--config", "extra.name=hello")
	root := filepath.Dir(filepath.Dir(r.StateDir))
	raw, err := os.ReadFile(filepath.Join(root, "home", ".config", "gx", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Server struct {
			RemoteControl bool `json:"remote-control"`
			WatchConfig   bool `json:"watch-config"`
		} `json:"server"`
		Extra struct {
			Name string `json:"name"`
		} `json:"extra"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Server.RemoteControl || cfg.Extra.Name != "hello" {
		t.Errorf("config = %s, want server.remote-control true and extra.name hello", raw)
	}
	r.stop(t)
}

const designAddr = "demo:design-epic"

func (r *running) currentStep(t *testing.T) server.ProgressStep {
	t.Helper()
	var d server.EpicDetail
	if err := json.Unmarshal([]byte(r.gx(t, "server", "epics", "detail", designAddr, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	for _, s := range d.Steps {
		if s.State == server.MapStepCurrent {
			return s.Step
		}
	}
	t.Fatalf("no current step in %+v", d.Steps)
	return ""
}

// waitChat polls the session's chat until ok accepts it.
func (r *running) waitChat(t *testing.T, id string, ok func(server.SessionChatResponse) bool) server.SessionChatResponse {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var chat server.SessionChatResponse
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "chat", id, "--json")), &chat); err != nil {
			t.Fatal(err)
		}
		if ok(chat) {
			return chat
		}
		if time.Now().After(deadline) {
			t.Fatalf("chat never reached the wanted state; round = %+v next = %+v", chat.Round, chat.Next)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// waitStep polls until the epic detail shows step as current.
func (r *running) waitStep(t *testing.T, step server.ProgressStep) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for r.currentStep(t) != step {
		if time.Now().After(deadline) {
			t.Fatalf("current step = %q, want %q", r.currentStep(t), step)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// waitIdle waits for the session to end its turn: the scripted message is in
// the chat while the turn is still ending, and a prompt typed then is refused.
func (r *running) waitIdle(t *testing.T, id string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var list []server.SessionInfo
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
			t.Fatal(err)
		}
		for _, s := range list {
			if s.ID == id && s.Status.State == server.SessionIdle {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s never went idle", id)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func roundLabels(chat server.SessionChatResponse) []string {
	var labels []string
	for _, c := range chat.Round {
		labels = append(labels, c.Label)
	}
	return labels
}

func TestDesignScenario_SeededMapAndScriptedChat(t *testing.T) {
	t.Parallel()
	r := start(t, "design")
	if step := r.currentStep(t); step != server.ProgressResolveTickets {
		t.Errorf("current step = %q, want resolve-tickets", step)
	}
	// The verb moves the step the page shows.
	r.gx(t, "tickets", "set", designAddr, "--design-step", "spec")
	r.waitStep(t, server.ProgressSpec)
	repo := filepath.Join(filepath.Dir(filepath.Dir(r.StateDir)), "repo")
	tracked, err := exec.Command("git", "-C", repo, "ls-files").Output()
	if err != nil || strings.TrimSpace(string(tracked)) != agentfake.WorkerConfigFile {
		t.Errorf("trunk files = %q, %v; want only the worker config, so no spec", tracked, err)
	}

	var res server.SessionResult
	if err := json.Unmarshal([]byte(r.gx(t, "designs", "open", designAddr, "--json")), &res); err != nil || res.Session == nil {
		t.Fatalf("designs open = %+v, %v", res, err)
	}
	id := res.Session.ID

	chat := r.waitChat(t, id, func(c server.SessionChatResponse) bool { return len(c.Round) > 0 })
	if got := roundLabels(chat); !slices.Equal(got, []string{"8", "8b"}) {
		t.Fatalf("round labels = %v, want 8 and 8b", got)
	}

	r.promptWhenIdle(t, id, "SQLite, and yes to the migration")
	chat = r.waitChat(t, id, func(c server.SessionChatResponse) bool { return c.Next != nil })
	if len(chat.Next.Action.Action) == 0 {
		t.Errorf("next = %+v, want the Continue action", chat.Next)
	}

	// Continue as the page sends it; the CLI's run-action needs the session's
	// Actionable, which an answer typed by the server does not clear.
	r.waitIdle(t, id)
	r.postWhenIdle(t, id, "/v1/sessions/prompt", map[string]any{"id": id, "continue": true, "last_message": chat.Messages[len(chat.Messages)-1].ID})
	r.waitChat(t, id, func(c server.SessionChatResponse) bool { return slices.Equal(roundLabels(c), []string{"9"}) })
	r.stop(t)
}

func TestDesignTwoLiveScenario_TwoSessionsNewestPlaysRoundOne(t *testing.T) {
	r := start(t, "design-two-live")
	var list []server.SessionInfo
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	var live []server.SessionInfo
	for _, s := range list {
		if s.Ticket == designAddr && s.Design {
			live = append(live, s)
		}
	}
	if len(live) != 2 {
		t.Fatalf("design sessions of %s = %+v, want 2", designAddr, list)
	}
	newest := live[0]
	if live[1].RunStartedAt.After(newest.RunStartedAt) {
		newest = live[1]
	}
	chat := r.waitChat(t, newest.ID, func(c server.SessionChatResponse) bool { return len(c.Round) > 0 })
	if got := roundLabels(chat); !slices.Equal(got, []string{"8", "8b"}) {
		t.Errorf("round labels = %v, want 8 and 8b", got)
	}
	r.stop(t)
}

// openDesign opens the design session of the scenario's map and returns its id.
func (r *running) openDesign(t *testing.T) string {
	t.Helper()
	var res server.SessionResult
	if err := json.Unmarshal([]byte(r.gx(t, "designs", "open", designAddr, "--json")), &res); err != nil || res.Session == nil {
		t.Fatalf("designs open = %+v, %v", res, err)
	}
	return res.Session.ID
}

// promptWhenIdle types text until it shows in the chat. The status the server
// reports lags the fake's turn, so a prompt sent right after a scripted message
// can be refused, or typed into a busy session and lost.
func (r *running) promptWhenIdle(t *testing.T, id, text string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		cmd := exec.Command(gxBin, "agents", "prompt", id, text)
		cmd.Env = r.Env
		if _, err := cmd.CombinedOutput(); err == nil {
			for range 10 {
				var chat server.SessionChatResponse
				if err := json.Unmarshal([]byte(r.gx(t, "agents", "chat", id, "--json")), &chat); err != nil {
					t.Fatal(err)
				}
				for _, m := range chat.Messages {
					if m.Role == transcript.ChatRolePerson && m.Text == text {
						return
					}
				}
				time.Sleep(200 * time.Millisecond)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("session %s never took the prompt %q", id, text)
}

// agentTexts are the text of the session's assistant messages, oldest first.
func agentTexts(chat server.SessionChatResponse) []string {
	var texts []string
	for _, m := range chat.Messages {
		if m.Role == transcript.ChatRoleAgent && m.Kind == transcript.ChatKindText {
			texts = append(texts, m.Text)
		}
	}
	return texts
}

func TestDesignScripts_FakeAgentPlaysTheTurns(t *testing.T) {
	t.Parallel()
	const runEnd = "scripted run is over"
	tests := []struct {
		scenario string
		// want is the text each assistant message must contain, in order; the
		// test answers every message but the last.
		want []string
	}{
		{"design-plain", []string{"Q8", "thinking it over"}},
		{"design-branch", []string{runEnd}},
		{"design-error", []string{"Q8", "Error: the spec step failed"}},
		{"design-guard", slices.Repeat([]string{runEnd}, 6)},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			t.Parallel()
			// A long countdown keeps the server from answering run-end lines itself.
			r := startWith(t, tt.scenario, nil, "--auto-advance-delay", "1h")
			id := r.openDesign(t)
			for i, want := range tt.want {
				r.waitChat(t, id, func(c server.SessionChatResponse) bool {
					texts := agentTexts(c)
					return len(texts) == i+1 && strings.Contains(texts[i], want)
				})
				if i < len(tt.want)-1 {
					r.promptWhenIdle(t, id, fmt.Sprintf("go on (%d)", i))
				}
			}
			r.stop(t)
		})
	}
}

// designSessions polls `gx agents list` until every ticket has a design session
// that satisfies ok, and returns them by ticket.
func (r *running) designSessions(t *testing.T, tickets []string, ok func(server.SessionInfo) bool) map[string]server.SessionInfo {
	t.Helper()
	var list []server.SessionInfo
	got := map[string]server.SessionInfo{}
	if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
		list = nil
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
			t.Fatal(err)
		}
		got = map[string]server.SessionInfo{}
		for _, s := range list {
			if s.Design && slices.Contains(tickets, s.Ticket) && ok(s) {
				got[s.Ticket] = s
			}
		}
		return len(got) == len(tickets)
	}) {
		t.Fatalf("design sessions of %v = %+v", tickets, list)
	}
	return got
}

// launchModels is the --model of every fake agent started so far, sorted. The
// fake logs its argv under the session's home.
func (r *running) launchModels(t *testing.T) []string {
	t.Helper()
	home := filepath.Join(filepath.Dir(filepath.Dir(r.StateDir)), "home")
	paths, _ := filepath.Glob(agentfake.TUIArgsPath(home, "*"))
	var models []string
	for _, p := range paths {
		args, err := agentfake.ReadTUIArgs(home, strings.TrimSuffix(filepath.Base(p), ".args"))
		if err != nil {
			t.Fatal(err)
		}
		if i := slices.Index(args, "--model"); i >= 0 {
			models = append(models, args[i+1])
		}
	}
	slices.Sort(models)
	return models
}

// waitLaunchModels polls until the fakes started so far launched under want.
func (r *running) waitLaunchModels(t *testing.T, want []string) {
	t.Helper()
	var got []string
	if testutil.Poll(20*time.Second, 200*time.Millisecond, func() bool {
		got = r.launchModels(t)
		return slices.Equal(got, want)
	}) {
		return
	}
	t.Fatalf("launch models = %v, want %v", got, want)
}

func TestDesignModelTiersScenario_TierPerMapAndLaunchArgs(t *testing.T) {
	r := start(t, "design-model-tiers")
	want := map[string]string{
		"demo:tier-chart":       "strongest",
		"demo:tier-later":       "worker",
		"demo:tier-unconfirmed": "strongest",
		"demo:tier-no-step":     "strongest",
	}
	tickets := slices.Collect(maps.Keys(want))
	got := r.designSessions(t, tickets, func(server.SessionInfo) bool { return true })
	for ticket, tier := range want {
		if string(got[ticket].Tier) != tier {
			t.Errorf("%s tier = %q, want %q", ticket, got[ticket].Tier, tier)
		}
	}
	r.waitLaunchModels(t, []string{"opus", "opus", "opus", "sonnet"})
	// The session list shows the model the fake reports on its replies.
	r.designSessions(t, tickets, func(s server.SessionInfo) bool {
		return s.Model == map[string]string{"strongest": "opus", "worker": "sonnet"}[string(s.Tier)]
	})
	r.stop(t)
}

func TestChartingHandoffScenario_ContinueRestartsOnWorkerOrFailsOnStaleSlug(t *testing.T) {
	r := start(t, "charting-handoff")
	tickets := []string{"demo:handoff-auto", "demo:handoff-manual", "stale:handoff-stale"}
	sessions := r.designSessions(t, tickets, func(s server.SessionInfo) bool { return s.Tier == "strongest" })
	if !sessions[tickets[0]].AutoAdvance {
		t.Errorf("%s auto-advance is off, want on", tickets[0])
	}
	for _, ticket := range tickets[1:] {
		id := sessions[ticket].ID
		if sessions[ticket].AutoAdvance {
			t.Errorf("%s auto-advance is on, want off", ticket)
		}
		chat := r.waitChat(t, id, func(c server.SessionChatResponse) bool { return len(c.Messages) > 0 })
		r.postWhenIdle(t, id, "/v1/sessions/prompt", map[string]any{"id": id, "continue": true, "last_message": chat.Messages[len(chat.Messages)-1].ID})
	}
	// The auto-advance countdown restarts the first session by itself.
	r.designSessions(t, tickets[:2], func(s server.SessionInfo) bool { return s.Tier == "worker" })
	r.designSessions(t, tickets[2:], func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
	if models := r.launchModels(t); !slices.Contains(models, agentfake.StaleModel) {
		t.Errorf("launch models = %v, want the stale one among them", models)
	}
	r.stop(t)
}
