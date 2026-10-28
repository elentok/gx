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
	log := ""
	if withLog {
		log = nativerunner.OutFile
	}
	return writeRunnerDir(t, stateDir, "headless", log)
}

// writeRunnerDir lays out runner's agent directory for addr with log, or no
// log when empty.
func writeRunnerDir(t *testing.T, stateDir, runner, log string) string {
	t.Helper()
	label, _, _ := ralphloop.IterationIdentity(addr.Epic, addr.ID, "")
	dir := filepath.Join(nativerunner.AgentsRoot(stateDir, addr.Project), label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(nativerunner.Meta{
		Runner: runner, PID: 42, PIDStart: "t0", HolderPID: 41, HolderPIDStart: "t0",
		SessionID: session, Label: label, Cwd: "/work/tree", Offset: 17,
	})
	if err := os.WriteFile(filepath.Join(dir, nativerunner.MetaFile), meta, 0o600); err != nil {
		t.Fatal(err)
	}
	if log != "" {
		if err := os.WriteFile(filepath.Join(dir, log), nil, 0o600); err != nil {
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
	if l.Path != filepath.Join(dir, nativerunner.OutFile) || l.Offset != 17 || l.Pruned || l.PtyDir != "" {
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
	stateDir := t.TempDir()
	writeAgentDir(t, stateDir, false)
	l, err := agentlog.Locator{StateDir: stateDir, Home: home, Procs: fakeProcs{}, Herdr: noHerdr(t)}.AgentLog(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if want := transcript.PathIn(home, "/work/tree", session); !l.Pruned || l.Transcript != want || l.Path != "" {
		t.Errorf("got %+v, want pruned with transcript %s", l, want)
	}
}

func TestAgentLog_PtyReadsTheTranscript(t *testing.T) {
	home := t.TempDir()
	stateDir := t.TempDir()
	dir := writeRunnerDir(t, stateDir, "pty", nativerunner.RawLogFile)
	procs := fakeProcs{
		41: {Start: "t0", Cmdline: "gx agent-holder --session-id " + session},
		42: liveProcs[42],
	}
	l, err := agentlog.Locator{StateDir: stateDir, Home: home, Procs: procs, Herdr: noHerdr(t)}.AgentLog(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if want := transcript.PathIn(home, "/work/tree", session); l.Path != want || l.Offset != 0 || l.Pruned || l.PtyDir != dir {
		t.Errorf("got %+v, want transcript %s from offset 0", l, want)
	}
	if l.Ended() {
		t.Error("live pty agent reported ended")
	}
	delete(procs, 41)
	if !l.Ended() {
		t.Error("pty agent with its holder gone not reported ended")
	}
}

func TestAgentLog_TranscriptUnderAnotherProjectDir(t *testing.T) {
	home := t.TempDir()
	stateDir := t.TempDir()
	writeRunnerDir(t, stateDir, "pty", nativerunner.RawLogFile)
	// The launcher wrote the transcript under the epic's dir, not the worktree's.
	want := transcript.PathIn(home, "/work", session)
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := agentlog.Locator{StateDir: stateDir, Home: home, Procs: fakeProcs{}, Herdr: noHerdr(t)}.AgentLog(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if l.Path != want {
		t.Errorf("got %s, want %s", l.Path, want)
	}
}

func TestAgentLog_HerdrTranscriptUnderAnotherProjectDir(t *testing.T) {
	home := t.TempDir()
	want := transcript.PathIn(home, "/work", session)
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	derived := transcript.PathIn(home, "/work/tree", session)
	loc := agentlog.Locator{StateDir: t.TempDir(), Herdr: herdrWith(agentlog.HerdrIteration{Agent: "claude", Transcript: derived}, true)}
	l, err := loc.AgentLog(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if l.Path != want {
		t.Errorf("got %s, want %s", l.Path, want)
	}
}

func TestAgentLog_PtyPruned(t *testing.T) {
	home := t.TempDir()
	stateDir := t.TempDir()
	dir := writeRunnerDir(t, stateDir, "pty", "")
	l, err := agentlog.Locator{StateDir: stateDir, Home: home, Procs: fakeProcs{}, Herdr: noHerdr(t)}.AgentLog(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if want := transcript.PathIn(home, "/work/tree", session); !l.Pruned || l.Transcript != want || l.PtyDir != dir {
		t.Errorf("got %+v, want pruned with transcript %s", l, want)
	}
}

func TestAgentLog_UnknownRunner(t *testing.T) {
	stateDir := t.TempDir()
	dir := writeAgentDir(t, stateDir, true)
	meta, _ := json.Marshal(nativerunner.Meta{Runner: "martian", SessionID: session})
	if err := os.WriteFile(filepath.Join(dir, nativerunner.MetaFile), meta, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := agentlog.Locator{StateDir: stateDir, Procs: fakeProcs{}, Herdr: noHerdr(t)}.AgentLog(context.Background(), addr)
	if !errors.Is(err, nativerunner.ErrUnknownRunner) {
		t.Errorf("got %v, want ErrUnknownRunner", err)
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
	// The transcript lives under a temp dir: transcript.Resolve globs the grandparent of a missing path, and
	// a path like /t/s.jsonl would make it glob the filesystem root (it hangs opening /home on macOS).
	transcriptPath := filepath.Join(t.TempDir(), "projects", "p", "s.jsonl")
	loc := agentlog.Locator{StateDir: t.TempDir(), Herdr: herdrWith(agentlog.HerdrIteration{Agent: "claude", Transcript: transcriptPath}, true)}
	l, err := loc.AgentLog(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if l.Path != transcriptPath || l.Offset != 0 || l.Ended() {
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
