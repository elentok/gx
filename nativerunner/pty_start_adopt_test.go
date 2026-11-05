package nativerunner_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/testutil"
)

// secondRunner is a fresh runner over p's root, like a restarted server.
func secondRunner(t *testing.T, p *nativerunner.Pty) *nativerunner.Pty {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	q := ptyOn(p.Root, exe)
	q.Env = p.Env
	q.Claude = p.Claude
	return q
}

func readHolderMeta(t *testing.T, p *nativerunner.Pty, label string) nativerunner.Meta {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(p.Root, label, nativerunner.MetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var meta nativerunner.Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
}

// snapshotDir maps each file in label's dir to its bytes.
func snapshotDir(t *testing.T, p *nativerunner.Pty, label string) map[string]string {
	t.Helper()
	dir := filepath.Join(p.Root, label)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		info, err := os.Lstat(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSocket != 0 {
			out[e.Name()] = "socket"
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(b)
	}
	return out
}

func TestPty_StartAdoptsSameDirHolder(t *testing.T) {
	t.Parallel()
	p := fakeclaudeRunner(t)
	cwd := t.TempDir()
	first, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: cwd, Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop(first) })
	before := readHolderMeta(t, p, "e-01")

	q := secondRunner(t, p)
	got, err := q.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: cwd, Kind: "claude"})
	if err != nil {
		t.Fatalf("Start = %v; want the live holder adopted", err)
	}
	if got.SessionID != first.SessionID {
		t.Errorf("SessionID = %q; want the existing %q", got.SessionID, first.SessionID)
	}
	if after := readHolderMeta(t, p, "e-01"); after.HolderPID != before.HolderPID {
		t.Errorf("holder pid %d -> %d; want no second claude", before.HolderPID, after.HolderPID)
	}
}

func TestPty_StartLiveHolderIsLabelTaken(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	tests := []struct {
		name string
		opts agentrunner.StartOptions
	}{
		{"NoAdopt in the same dir", agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: cwd, Kind: "claude", NoAdopt: true}},
		{"another dir", agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := fakeclaudeRunner(t)
			first, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: cwd, Kind: "claude"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = p.Stop(first) })
			before := snapshotDir(t, p, "e-01")

			_, err = secondRunner(t, p).Start(tt.opts)
			if !errors.Is(err, agentrunner.ErrLabelTaken) {
				t.Fatalf("Start = %v; want ErrLabelTaken", err)
			}
			var taken *agentrunner.LabelTakenError
			if !errors.As(err, &taken) || taken.HolderCwd != cwd {
				t.Errorf("error = %+v; want a LabelTakenError with cwd %s", taken, cwd)
			}
			if after := snapshotDir(t, p, "e-01"); !reflect.DeepEqual(before, after) {
				t.Errorf("holder files changed:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

func TestPty_StartCleansDeadHolderDir(t *testing.T) {
	t.Parallel()
	p := fakeclaudeRunner(t)
	cwd := t.TempDir()
	first, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: cwd, Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	meta := readHolderMeta(t, p, "e-01")
	_ = syscall.Kill(meta.PID, syscall.SIGKILL)
	_ = syscall.Kill(meta.HolderPID, syscall.SIGKILL)
	_ = first
	waitGone(t, meta.HolderPID)

	got, err := secondRunner(t, p).Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: cwd, Kind: "claude", NoAdopt: true})
	if err != nil {
		t.Fatalf("Start over a dead holder = %v", err)
	}
	if got.SessionID == first.SessionID {
		t.Error("Start reused the dead session")
	}
}

func TestPty_StartHolderThatCannotBindShowsItsStderr(t *testing.T) {
	t.Parallel()
	p := fakeclaudeRunner(t)
	// A socket path over the unix limit makes the holder's listen fail.
	label := strings.Repeat("l", 150)
	_, err := p.Start(agentrunner.StartOptions{Label: label, Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err == nil {
		t.Fatal("Start = nil; want the holder's bind failure")
	}
	if strings.Contains(err.Error(), "holder.sock never answered") {
		t.Errorf("error = %q; want the holder's own stderr", err)
	}
	stderr, rerr := os.ReadFile(filepath.Join(p.Root, label, holder.HolderErrFile))
	if rerr != nil {
		t.Fatalf("holder.err: %v", rerr)
	}
	if len(stderr) == 0 || !strings.Contains(err.Error(), strings.TrimSpace(string(stderr))) {
		t.Errorf("error = %q; want it to carry holder.err %q", err, stderr)
	}
}

func TestPty_StartSilentHolderIsTakenAndStopKillsIt(t *testing.T) {
	t.Parallel()
	p := fakeclaudeRunner(t)
	cwd := t.TempDir()
	if _, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: cwd, Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	meta := readHolderMeta(t, p, "e-01")
	// A stopped holder keeps its pid and accepts connections but never answers.
	if err := syscall.Kill(meta.HolderPID, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-meta.HolderPID, syscall.SIGKILL) })

	q := secondRunner(t, p)
	_, err := q.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: cwd, Kind: "claude"})
	if !errors.Is(err, agentrunner.ErrLabelTaken) {
		t.Fatalf("Start = %v; want ErrLabelTaken", err)
	}
	s, ok, err := q.Find("e-01")
	if err != nil || !ok {
		t.Fatalf("Find = %v, %v; want the silent holder", ok, err)
	}
	if err := q.Stop(s); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	waitGone(t, meta.HolderPID)
}

// A died session stays findable while its holder runs, so start-over can stop
// it, and is forgotten once the holder is gone.
func TestPty_FindForgetsDiedSessionOnceHolderIsGone(t *testing.T) {
	t.Parallel()
	p := fakeclaudeRunner(t)
	cwd := t.TempDir()
	if _, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: cwd, Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	meta := readHolderMeta(t, p, "e-01")
	if err := syscall.Kill(meta.HolderPID, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-meta.HolderPID, syscall.SIGKILL) })

	q := secondRunner(t, p)
	if _, ok, err := q.Find("e-01"); err != nil || !ok {
		t.Fatalf("Find = %v, %v; want the silent holder", ok, err)
	}
	_ = syscall.Kill(-meta.HolderPID, syscall.SIGKILL)
	waitGone(t, meta.HolderPID)
	if _, ok, err := q.Find("e-01"); err != nil || ok {
		t.Fatalf("Find after the holder died = %v, %v; want it forgotten", ok, err)
	}
}

func waitGone(t *testing.T, pid int) {
	t.Helper()
	if !testutil.Poll(5*time.Second, 20*time.Millisecond, func() bool { return syscall.Kill(pid, 0) != nil }) {
		t.Fatalf("pid %d still runs", pid)
	}
}
