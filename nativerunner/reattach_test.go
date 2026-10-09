package nativerunner_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
)

type fakeProcs struct {
	mu    sync.Mutex
	procs map[int]nativerunner.Process
}

func (f *fakeProcs) Lookup(pid int) (nativerunner.Process, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.procs[pid]
	return p, ok, nil
}

func (f *fakeProcs) kill(pid int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.procs, pid)
}

const (
	agentPID   = 4242
	agentStart = "Mon Oct  6 10:00:00 2026"
	agentSID   = "sid-1"
)

func agentProcs() *fakeProcs {
	return &fakeProcs{procs: map[int]nativerunner.Process{
		agentPID: {Start: agentStart, Cmdline: "claude -p --session-id " + agentSID},
	}}
}

// prepareAgent writes an agent directory as a previous server left it:
// before is an earlier run's output, after is this session's.
func prepareAgent(t *testing.T, root, label, before, after string) string {
	t.Helper()
	dir := filepath.Join(root, label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := nativerunner.Meta{
		Runner: "headless", PID: agentPID, PIDStart: agentStart, SessionID: agentSID,
		Label: label, Epic: "epic", Offset: int64(len(before)),
	}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, nativerunner.MetaFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nativerunner.OutFile), []byte(before+after), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func lines(ls ...string) string { return strings.Join(ls, "\n") + "\n" }

const (
	evAssistant = `{"type":"assistant","uuid":"a1","message":{}}`
	evResult    = `{"type":"result","uuid":"r1","subtype":"success"}`
	evQueued    = `{"type":"command_lifecycle","command_uuid":"c2","state":"queued"}`
)

func TestReattach_DeadAgentVerdicts(t *testing.T) {
	reused := map[int]nativerunner.Process{agentPID: {Start: "Tue Oct  7 09:00:00 2026", Cmdline: "claude -p --session-id " + agentSID}}
	otherCmd := map[int]nativerunner.Process{agentPID: {Start: agentStart, Cmdline: "vim notes.md"}}
	tests := []struct {
		name          string
		procs         map[int]nativerunner.Process
		before, after string
		want          nativerunner.Verdict
	}{
		{name: "ends with result", after: lines(evAssistant, evResult), want: nativerunner.VerdictFinished},
		{name: "died mid-turn", after: lines(evAssistant), want: nativerunner.VerdictDied},
		{name: "no output", want: nativerunner.VerdictDied},
		{name: "result then partial line", after: lines(evAssistant, evResult) + `{"type":"assis`, want: nativerunner.VerdictFinished},
		{name: "partial result line", after: lines(evAssistant) + evResult, want: nativerunner.VerdictDied},
		{name: "result before offset is an earlier run", before: lines(evResult), after: lines(`{"type":"assistant","uuid":"a2"}`), want: nativerunner.VerdictDied},
		{name: "prompt queued after result", after: lines(evAssistant, evResult, evQueued), want: nativerunner.VerdictDied},
		{name: "pid reused, start time differs", procs: reused, after: lines(evAssistant), want: nativerunner.VerdictDied},
		{name: "pid reused, session id missing", procs: otherCmd, after: lines(evAssistant), want: nativerunner.VerdictDied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			prepareAgent(t, root, "epic-10", tt.before, tt.after)
			r := &nativerunner.Headless{Root: root, Procs: &fakeProcs{procs: tt.procs}}
			_, got, err := r.Reattach("epic-10")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("verdict = %v, want %v", got, tt.want)
			}
			if _, found, _ := r.Find("epic-10"); found {
				t.Error("a dead agent was adopted")
			}
		})
	}
}

func TestReattach_MissingDirIsNotFound(t *testing.T) {
	r := &nativerunner.Headless{Root: t.TempDir(), Procs: agentProcs()}
	if _, _, err := r.Reattach("epic-10"); !errors.Is(err, agentrunner.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestReattach_LiveAgentReplaysIdempotentlyByUUID(t *testing.T) {
	root := t.TempDir()
	dir := prepareAgent(t, root, "epic-10", lines(evResult),
		lines(evAssistant, evResult, evAssistant, evResult)+`{"type":"assistant","uuid":"a9"`)
	if err := syscall.Mkfifo(filepath.Join(dir, nativerunner.StdinFile), 0o600); err != nil {
		t.Fatal(err)
	}
	procs := agentProcs()
	r := &nativerunner.Headless{Root: root, Procs: procs, PollInterval: 5 * time.Millisecond}

	s, verdict, err := r.Reattach("epic-10")
	if err != nil || verdict != nativerunner.VerdictLive {
		t.Fatalf("Reattach = %v, %v; want live", verdict, err)
	}
	if found, ok, _ := r.Find("epic-10"); !ok || found != s {
		t.Fatalf("Find = %+v, %v; want %+v", found, ok, s)
	}
	if s.SessionID != agentSID {
		t.Errorf("session id = %q, want %q", s.SessionID, agentSID)
	}
	if st, _ := r.Status(s); st.Turn != 1 || st.State != agentrunner.StateIdle {
		t.Errorf("status after replay = %+v, want turn 1 idle", st)
	}
	if again, verdict, _ := r.Reattach("epic-10"); verdict != nativerunner.VerdictLive || again != s {
		t.Errorf("second Reattach = %+v %v, want the adopted session", again, verdict)
	}

	procs.kill(agentPID)
	if st, err := r.Wait(s, []agentrunner.State{agentrunner.StateDone}, 5*time.Second); err != nil {
		t.Fatalf("Wait done after exit: %v (status %+v)", err, st)
	}
}

func TestHeadless_RestartedServerReattachesLiveClaude(t *testing.T) {
	h := newHarness(t)
	s := h.start(t, "epic-10")
	if err := h.runner.Prompt(s, "hi"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second); err != nil {
		t.Fatal(err)
	}

	restarted := &nativerunner.Headless{
		Root: h.root, Claude: h.runner.Claude, PollInterval: 5 * time.Millisecond,
		StopGrace: 2 * time.Second, PromptTimeout: 5 * time.Second,
	}
	got, verdict, err := restarted.Reattach("epic-10")
	if err != nil || verdict != nativerunner.VerdictLive {
		t.Fatalf("Reattach = %v, %v; want live", verdict, err)
	}
	if got.SessionID != s.SessionID {
		t.Errorf("session id = %q, want %q", got.SessionID, s.SessionID)
	}
	if st, err := restarted.Status(got); err != nil || st.Turn != 1 || st.State != agentrunner.StateIdle {
		t.Fatalf("status after reattach = %+v, %v; want idle turn 1", st, err)
	}
	if err := restarted.Prompt(got, "again"); err != nil {
		t.Fatalf("Prompt after reattach: %v", err)
	}
	if st, err := restarted.Wait(got, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second); err != nil || st.Turn != 2 {
		t.Fatalf("status after prompt = %+v, %v; want idle turn 2", st, err)
	}

	if err := restarted.Stop(got); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(h.root, "epic-10")
	if _, err := os.Stat(filepath.Join(dir, nativerunner.StdinFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stdin FIFO left behind after Stop: %v", err)
	}
	for _, kept := range []string{nativerunner.MetaFile, nativerunner.OutFile} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Errorf("%s removed by Stop: %v", kept, err)
		}
	}
	if _, verdict, err := restarted.Reattach("epic-10"); err != nil || verdict == nativerunner.VerdictLive {
		t.Errorf("Reattach after Stop = %v, %v; want dead", verdict, err)
	}
}

func TestHeadless_StartRecordsIdentityAtALineBoundary(t *testing.T) {
	h := newHarness(t)
	dir := filepath.Join(h.root, "epic-10")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	earlier := `{"type":"result"}` + "\n" + `{"type":"assis`
	if err := os.WriteFile(filepath.Join(dir, nativerunner.OutFile), []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}
	s := h.start(t, "epic-10")
	if err := h.runner.Prompt(s, "hi"); err != nil {
		t.Fatal(err)
	}
	if st, err := h.runner.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second); err != nil || st.Turn != 1 {
		t.Fatalf("status = %+v, %v; want idle turn 1", st, err)
	}

	data, err := os.ReadFile(filepath.Join(dir, nativerunner.MetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var meta nativerunner.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.PIDStart == "" {
		t.Error("meta has no pid start time")
	}
	if meta.Offset != int64(len(earlier))+1 {
		t.Errorf("offset = %d, want %d (after the ended partial line)", meta.Offset, len(earlier)+1)
	}
}

func TestHeadless_CleanupOfAMissingFIFOIsANoop(t *testing.T) {
	r := &nativerunner.Headless{Root: t.TempDir()}
	if err := r.Cleanup("epic-10"); err != nil {
		t.Fatal(err)
	}
}

func TestHeadless_PruneKeepsRecentParkedAndLiveAgents(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	old := now.Add(-nativerunner.DefaultLogRetention - time.Hour)
	recent := now.Add(-nativerunner.DefaultLogRetention + time.Hour)
	age := func(dir string, at time.Time) {
		for _, f := range []string{nativerunner.MetaFile, nativerunner.OutFile} {
			if err := os.Chtimes(filepath.Join(dir, f), at, at); err != nil {
				t.Fatal(err)
			}
		}
	}
	for label, at := range map[string]time.Time{"old": old, "parked": old, "recent": recent, "live": old} {
		dir := prepareAgent(t, root, label, "", lines(evAssistant))
		if label != "live" {
			// The fake process table only runs agentPID.
			meta := nativerunner.Meta{Runner: "headless", PID: agentPID + 1, PIDStart: agentStart, SessionID: agentSID}
			data, _ := json.Marshal(meta)
			if err := os.WriteFile(filepath.Join(dir, nativerunner.MetaFile), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		age(dir, at)
	}
	if err := os.MkdirAll(filepath.Join(root, "not-an-agent"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := &nativerunner.Headless{Root: root, Procs: agentProcs()}
	pruned, err := r.Prune(nativerunner.DefaultLogRetention, now, func(label string) bool { return label == "parked" })
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pruned, []string{"old"}) {
		t.Errorf("pruned = %q, want [old]", pruned)
	}
	for _, kept := range []string{"parked", "recent", "live", "not-an-agent"} {
		if _, err := os.Stat(filepath.Join(root, kept)); err != nil {
			t.Errorf("%s was pruned: %v", kept, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "old")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old survived: %v", err)
	}
}
