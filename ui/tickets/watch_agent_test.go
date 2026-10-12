package tickets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentlog"
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
