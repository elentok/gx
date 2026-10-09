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
	modal := watchModal{}.Open(loaded.title, loaded.lines, 120, 40)
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

func TestHasNativeAgent_FalseWithoutNativeLog(t *testing.T) {
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)
	if hasNativeAgent("gx:epic/01") {
		t.Error("expected no native agent")
	}
}
