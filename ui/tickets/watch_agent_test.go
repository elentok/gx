package tickets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/server"
)

const watchLogLine = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello from the agent"}]}}` + "\n"

func fakeNativeLog(t *testing.T, log agentlog.Log, err error) {
	t.Helper()
	prev := nativeAgentLog
	t.Cleanup(func() { nativeAgentLog = prev })
	nativeAgentLog = func(context.Context, string) (agentlog.Log, error) { return log, err }
}

func TestWatchAgent_RendersLogInModal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	if err := os.WriteFile(path, []byte(watchLogLine), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeNativeLog(t, agentlog.Log{Path: path}, nil)

	loaded, ok := cmdWatchAgent("gx:epic/01")().(watchLoadedMsg)
	if !ok {
		t.Fatal("expected watchLoadedMsg")
	}
	modal := watchModal{}.Open(loaded, 120, 40)
	if view := modal.View(); !strings.Contains(view, "hello from the agent") {
		t.Errorf("modal missing the rendered log:\n%s", view)
	}
}

func TestWatchAgent_HeadlessRefreshShowsNewLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	if err := os.WriteFile(path, []byte(watchLogLine), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeNativeLog(t, agentlog.Log{Path: path}, nil)

	modal := watchModal{}.Open(cmdWatchAgent("gx:epic/01")().(watchLoadedMsg), 120, 40)
	if view := modal.View(); !strings.Contains(view, "hello from the agent") {
		t.Fatalf("modal missing the rendered log:\n%s", view)
	}

	later := strings.Replace(watchLogLine, "hello from the agent", "a newer line", 1)
	if err := os.WriteFile(path, []byte(watchLogLine+later), 0o644); err != nil {
		t.Fatal(err)
	}
	modal, cmd := modal.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r returned no refresh command")
	}
	modal = modal.Open(cmd().(watchLoadedMsg), 120, 40)
	if view := modal.View(); !strings.Contains(view, "a newer line") {
		t.Errorf("refreshed modal missing the new line:\n%s", view)
	}
}

func TestWatchAgent_PtyShowsHolderScreenAndRefreshes(t *testing.T) {
	fakeNativeLog(t, agentlog.Log{Path: "transcript.jsonl", PtyDir: "/pty/dir"}, nil)
	prev := holderScreen
	t.Cleanup(func() { holderScreen = prev })
	screen := []string{"first screen"}
	var dirs []string
	holderScreen = func(dir string) ([]string, error) {
		dirs = append(dirs, dir)
		return screen, nil
	}

	modal := watchModal{}.Open(cmdWatchAgent("gx:epic/01")().(watchLoadedMsg), 120, 40)
	if view := modal.View(); !strings.Contains(view, "first screen") {
		t.Fatalf("modal missing the holder screen:\n%s", view)
	}

	screen = []string{"second screen"}
	modal, cmd := modal.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r returned no refresh command")
	}
	modal = modal.Open(cmd().(watchLoadedMsg), 120, 40)
	if view := modal.View(); !strings.Contains(view, "second screen") {
		t.Errorf("refreshed modal missing the new screen:\n%s", view)
	}
	if len(dirs) != 2 || dirs[0] != "/pty/dir" {
		t.Errorf("holder screen fetched from %q, want /pty/dir twice", dirs)
	}
}

func TestWatchAgent_PrunedLogPointsAtTranscript(t *testing.T) {
	fakeNativeLog(t, agentlog.Log{Pruned: true, Transcript: "/t/x.jsonl"}, nil)
	loaded := cmdWatchAgent("gx:epic/01")().(watchLoadedMsg)
	if got := strings.Join(loaded.lines, "\n"); !strings.Contains(got, "/t/x.jsonl") {
		t.Errorf("lines = %q", got)
	}
}

func TestWatch_NativeLive_OpensLogNotPane(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	if err := os.WriteFile(path, []byte(watchLogLine), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeNativeLog(t, agentlog.Log{Path: path, Ended: func() bool { return false }}, nil)

	cmd, ok := Model{}.serverAnswerActionCmd(actionsMenuResult{Action: actionWatch, Path: "gx:epic/01"})
	if !ok {
		t.Fatal("Watch not dispatched")
	}
	if _, ok := cmd().(watchLoadedMsg); !ok {
		t.Error("expected the native log modal")
	}
}

// failOnNativeLog fails the test if anything resolves native liveness before
// the returned undo runs, i.e. on the Update goroutine.
func failOnNativeLog(t *testing.T) (undo func()) {
	t.Helper()
	prev := nativeAgentLog
	nativeAgentLog = func(context.Context, string) (agentlog.Log, error) {
		t.Error("native liveness resolved on the Update goroutine")
		return agentlog.Log{}, agentlog.ErrNoAgent
	}
	return func() { nativeAgentLog = prev }
}

func TestWatch_ResolvesLivenessInCmd(t *testing.T) {
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)
	undo := failOnNativeLog(t)
	cmd, ok := Model{serverAPI: fakeServerAPI{}}.serverAnswerActionCmd(actionsMenuResult{Action: actionWatch, Path: "gx:epic/01"})
	undo()
	if !ok || cmd == nil {
		t.Fatal("Watch not dispatched")
	}
	cmd()
}

func TestMenuKey_ResolvesLivenessInCmd(t *testing.T) {
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)
	m := newServerModel(t).WithServer(fakeServerAPI{})
	m = withState(m, server.Snapshot{Tickets: []server.TicketInfo{{Address: "gx:alpha/01", Title: "First", Status: "claimed"}}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))

	undo := failOnNativeLog(t)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	undo()
	if updated.(Model).actionsMenu.IsOpen {
		t.Fatal("menu opened before liveness resolved")
	}
	if cmd == nil {
		t.Fatal("m issued no cmd")
	}
	updated, _ = updated.(Model).Update(cmd())
	if !updated.(Model).actionsMenu.IsOpen {
		t.Error("menu closed after liveness resolved, want open")
	}
}

func TestNativeAgentLive(t *testing.T) {
	alive := func() bool { return false }
	ended := func() bool { return true }
	cases := []struct {
		name string
		log  agentlog.Log
		err  error
		want bool
	}{
		{"no native agent", agentlog.Log{}, agentlog.ErrNoAgent, false},
		{"running", agentlog.Log{Path: "x", Ended: alive}, nil, true},
		{"ended", agentlog.Log{Path: "x", Ended: ended}, nil, false},
		{"pruned", agentlog.Log{Pruned: true}, nil, false},
	}
	for _, c := range cases {
		fakeNativeLog(t, c.log, c.err)
		if got := nativeAgentLive("gx:epic/01"); got != c.want {
			t.Errorf("%s: nativeAgentLive = %v, want %v", c.name, got, c.want)
		}
	}
}
