package agentlog_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

var addr = tickets.Address{Project: "proj", Epic: "epic", ID: "07"}

type fakeProcs map[int]nativerunner.Process

func (f fakeProcs) Lookup(pid int) (nativerunner.Process, bool, error) {
	p, ok := f[pid]
	return p, ok, nil
}

const session = "sess-1"

var liveProcs = fakeProcs{42: {Start: "t0", Cmdline: "claude --session-id " + session}}

// writeAgentDir lays out a native agent directory for addr; withLog false
// leaves it as Prune does.
func writeAgentDir(t *testing.T, stateDir string, withLog bool) string {
	t.Helper()
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, "")
	dir := filepath.Join(nativerunner.AgentsRoot(stateDir, addr.Project), label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(nativerunner.Meta{PID: 42, PIDStart: "t0", SessionID: session, Label: label, Cwd: "/work/tree", Offset: 17})
	if err := os.WriteFile(filepath.Join(dir, nativerunner.MetaFile), meta, 0o600); err != nil {
		t.Fatal(err)
	}
	if withLog {
		if err := os.WriteFile(filepath.Join(dir, nativerunner.OutFile), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func noHerdr(t *testing.T) func(context.Context, string) (agentlog.HerdrIteration, bool, error) {
	return func(context.Context, string) (agentlog.HerdrIteration, bool, error) {
		t.Error("herdr consulted although a native agent exists")
		return agentlog.HerdrIteration{}, false, nil
	}
}

func TestAgentLog_LiveNative(t *testing.T) {
	stateDir := t.TempDir()
	dir := writeAgentDir(t, stateDir, true)
	l, err := agentlog.Locator{StateDir: stateDir, Procs: liveProcs, Herdr: noHerdr(t)}.AgentLog(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if l.Path != filepath.Join(dir, nativerunner.OutFile) || l.Offset != 17 || l.Pruned {
		t.Errorf("got %+v", l)
	}
	if l.Ended() {
		t.Error("live agent reported ended")
	}
}

func TestAgentLog_FinishedNative(t *testing.T) {
	stateDir := t.TempDir()
	writeAgentDir(t, stateDir, true)
	l, err := agentlog.Locator{StateDir: stateDir, Procs: fakeProcs{}, Herdr: noHerdr(t)}.AgentLog(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Ended() {
		t.Error("finished agent not reported ended")
	}
}

func TestAgentLog_Pruned(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := t.TempDir()
	writeAgentDir(t, stateDir, false)
	l, err := agentlog.Locator{StateDir: stateDir, Procs: fakeProcs{}, Herdr: noHerdr(t)}.AgentLog(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if want := transcript.PathIn(home, "/work/tree", session); !l.Pruned || l.Transcript != want || l.Path != "" {
		t.Errorf("got %+v, want pruned with transcript %s", l, want)
	}
}

func herdrWith(it agentlog.HerdrIteration, ok bool) func(context.Context, string) (agentlog.HerdrIteration, bool, error) {
	return func(_ context.Context, address string) (agentlog.HerdrIteration, bool, error) {
		if address != addr.String() {
			return agentlog.HerdrIteration{}, false, nil
		}
		return it, ok, nil
	}
}

func TestAgentLog_HerdrClaude(t *testing.T) {
	loc := agentlog.Locator{StateDir: t.TempDir(), Herdr: herdrWith(agentlog.HerdrIteration{Agent: "claude", Transcript: "/t/s.jsonl"}, true)}
	l, err := loc.AgentLog(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if l.Path != "/t/s.jsonl" || l.Offset != 0 || l.Ended() {
		t.Errorf("got %+v (ended %v)", l, l.Ended())
	}
}

func TestAgentLog_HerdrCodex(t *testing.T) {
	loc := agentlog.Locator{StateDir: t.TempDir(), Herdr: herdrWith(agentlog.HerdrIteration{Agent: string(ralphloop.AgentCodex)}, true)}
	if _, err := loc.AgentLog(context.Background(), addr); !errors.Is(err, agentlog.ErrUnsupported) {
		t.Errorf("got %v, want ErrUnsupported", err)
	}
}

func TestAgentLog_None(t *testing.T) {
	for name, herdr := range map[string]func(context.Context, string) (agentlog.HerdrIteration, bool, error){
		"no herdr":      nil,
		"no live herdr": herdrWith(agentlog.HerdrIteration{}, false),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := agentlog.Locator{StateDir: t.TempDir(), Herdr: herdr}.AgentLog(context.Background(), addr)
			if !errors.Is(err, agentlog.ErrNoAgent) {
				t.Errorf("got %v, want ErrNoAgent", err)
			}
		})
	}
}
