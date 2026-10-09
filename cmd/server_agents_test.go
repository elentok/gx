package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentlog"
)

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// agentLogFixture copies the agentlog fixture behind a line from an earlier
// run, and returns a Log whose Offset skips that line.
func agentLogFixture(t *testing.T) agentlog.Log {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "agentlog", "testdata", "out.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	prev := `{"type":"user","message":{"role":"user","content":"previous run"}}` + "\n"
	p := filepath.Join(t.TempDir(), "out.jsonl")
	if err := os.WriteFile(p, append([]byte(prev), data...), 0o600); err != nil {
		t.Fatal(err)
	}
	return agentlog.Log{Path: p, Offset: int64(len(prev)), Ended: func() bool { return true }}
}

func watchOutput(t *testing.T, log agentlog.Log, opts agentsWatchOpts) []string {
	t.Helper()
	var out bytes.Buffer
	if err := runAgentsWatch(context.Background(), log, opts, &out); err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
}

func assertLines(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAgentsWatch_Dump(t *testing.T) {
	golden := readLines(t, filepath.Join("..", "agentlog", "testdata", "out.golden"))
	assertLines(t, watchOutput(t, agentLogFixture(t), agentsWatchOpts{}), golden)
}

func TestAgentsWatch_Tail(t *testing.T) {
	golden := readLines(t, filepath.Join("..", "agentlog", "testdata", "out.golden"))
	assertLines(t, watchOutput(t, agentLogFixture(t), agentsWatchOpts{Tail: 3}), golden[len(golden)-3:])
}

func TestAgentsWatch_JSON(t *testing.T) {
	raw := readLines(t, filepath.Join("..", "agentlog", "testdata", "out.jsonl"))
	assertLines(t, watchOutput(t, agentLogFixture(t), agentsWatchOpts{JSON: true}), raw)
	assertLines(t, watchOutput(t, agentLogFixture(t), agentsWatchOpts{JSON: true, Tail: 2}), raw[len(raw)-2:])
}

func TestAgentsWatch_FollowExitsWhenAgentEnded(t *testing.T) {
	golden := readLines(t, filepath.Join("..", "agentlog", "testdata", "out.golden"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out bytes.Buffer
	if err := runAgentsWatch(ctx, agentLogFixture(t), agentsWatchOpts{Follow: true}, &out); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("follow did not exit on its own for an ended agent")
	}
	assertLines(t, strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n"), golden)
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestAgentsWatch_FollowStreamsUntilEnd(t *testing.T) {
	log := agentLogFixture(t)
	var ended atomic.Bool
	log.Ended = ended.Load
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out syncBuffer
	done := make(chan error, 1)
	go func() { done <- runAgentsWatch(ctx, log, agentsWatchOpts{Follow: true, Tail: 1}, &out) }()

	waitFor(t, ctx, func() bool { return strings.Contains(out.String(), "[result] error") })
	f, err := os.OpenFile(log.Path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Split across writes so follow has to join a partial line.
	_, _ = f.WriteString(`{"type":"result","subtype":"success",`)
	time.Sleep(2 * logFollowInterval)
	_, _ = f.WriteString(`"result":"late"}` + "\n")
	f.Close()
	waitFor(t, ctx, func() bool { return strings.Contains(out.String(), "late") })
	ended.Store(true)

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("follow did not stop after the agent ended")
	}
	assertLines(t, strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n"),
		[]string{"[result] error: error_during_execution", "[result] late"})
}

func TestAgentsWatch_Pruned(t *testing.T) {
	var out bytes.Buffer
	log := agentlog.Log{Pruned: true, Transcript: "/home/x/.claude/projects/p/s.jsonl"}
	if err := runAgentsWatch(context.Background(), log, agentsWatchOpts{Follow: true}, &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "log pruned; transcript: /home/x/.claude/projects/p/s.jsonl\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
