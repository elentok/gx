package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/gocachegate"
)

// gateWait is how long a test waits for something to happen; gateQuiet is how
// long it watches for something that must not. The scan sees every `go` process
// on the machine, so the wait is generous.
const (
	gateWait  = 60 * time.Second
	gateQuiet = 1500 * time.Millisecond
)

func (r *running) diskFree(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.DiskDir, "free"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

// waitFree waits for the free file to read want.
func (r *running) waitFree(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(gateWait)
	for time.Now().Before(deadline) {
		if r.diskFree(t) == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("free = %s, want %s", r.diskFree(t), want)
}

// stayFree fails if the free file changes while the gate should hold the clean.
func (r *running) stayFree(t *testing.T, want string) {
	t.Helper()
	time.Sleep(gateQuiet)
	if got := r.diskFree(t); got != want {
		t.Fatalf("free = %s, want %s: the clean ran while the gate was held", got, want)
	}
}

// cleanNow runs "Clean now" in the background and returns its output channel.
func (r *running) cleanNow() <-chan string {
	out := make(chan string, 1)
	go func() {
		cmd := exec.Command(gxBin, "server", "disk", "clean", "fixture disk")
		cmd.Env = r.Env
		b, _ := cmd.CombinedOutput()
		out <- string(b)
	}()
	return out
}

func TestCacheGate_CleanNowWaitsForARegisteredBuild(t *testing.T) {
	r := start(t, "cache-gate")
	defer r.stop(t)
	r.setDisk(t, "free", "15G")
	r.waitDiskStatus(t, "fixture disk: 15G free")
	r.setDisk(t, "clean", "25G")

	release := gocachegate.Register(r.StateDir)
	done := r.cleanNow()
	r.stayFree(t, "15G")
	release()

	select {
	case out := <-done:
		if !strings.Contains(out, "25G free now") {
			t.Errorf("clean output = %q, want 25G free now", out)
		}
	case <-time.After(gateWait):
		t.Fatal("Clean now never ended after the build ended")
	}
}

func TestCacheGate_TheAutomaticCleanWaitsForARegisteredBuild(t *testing.T) {
	r := start(t, "cache-gate")
	defer r.stop(t)
	r.setDisk(t, "clean", "25G")

	release := gocachegate.Register(r.StateDir)
	r.setDisk(t, "free", "5G")
	r.stayFree(t, "5G")
	release()

	r.waitFree(t, "25G")
}

func TestCacheGate_ACustomCleanupCommandWaitsToo(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "cleaned")
	script := filepath.Join(t.TempDir(), "clean.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho ran > "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := startWith(t, "cache-gate-custom", nil, "--config", `disk-space.cleanup-command=["`+script+`"]`)
	defer r.stop(t)
	r.setDisk(t, "free", "15G")
	r.waitDiskStatus(t, "fixture disk: 15G free")

	release := gocachegate.Register(r.StateDir)
	done := r.cleanNow()
	time.Sleep(gateQuiet)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the custom command ran while the gate was held")
	}
	release()

	select {
	case <-done:
	case <-time.After(gateWait):
		t.Fatal("Clean now never ended after the build ended")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the custom command never ran: %v", err)
	}
}

func TestCacheGate_AGoProcessHoldsTheClean(t *testing.T) {
	r := start(t, "cache-gate")
	defer r.stop(t)
	r.setDisk(t, "free", "15G")
	r.waitDiskStatus(t, "fixture disk: 15G free")
	r.setDisk(t, "clean", "25G")

	hold := filepath.Join(r.GateDir, "go-hold")
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	goProc := exec.Command(filepath.Join(r.GateDir, "bin", "go"), "build")
	if err := goProc.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = goProc.Process.Kill()
			_ = goProc.Wait()
		}
	}()
	started := filepath.Join(r.GateDir, "go-started")
	for deadline := time.Now().Add(gateWait); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the fake go never started")
		}
	}

	done := r.cleanNow()
	r.stayFree(t, "15G")
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	waited = true
	if err := goProc.Wait(); err != nil {
		t.Fatal(err)
	}

	select {
	case out := <-done:
		if !strings.Contains(out, "25G free now") {
			t.Errorf("clean output = %q, want 25G free now", out)
		}
	case <-time.After(gateWait):
		t.Fatal("Clean now never ended after the go process ended")
	}
}

func TestCacheGate_GoCleanCacheWithTheServerStoppedWaitsForARegisteredBuild(t *testing.T) {
	r := start(t, "cache-gate")
	defer r.stop(t)
	// Only the server stops; the fixture keeps its state dir and fake go.
	r.gx(t, "server", "stop")

	release := gocachegate.Register(r.StateDir)
	cmd := exec.Command(gxBin, "brief", "go-clean-cache")
	cmd.Dir = t.TempDir()
	cmd.Env = append(append([]string{}, r.Env...), "PATH="+filepath.Join(r.GateDir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	done := make(chan string, 1)
	go func() {
		b, _ := cmd.CombinedOutput()
		done <- string(b)
	}()

	time.Sleep(gateQuiet)
	logPath := filepath.Join(r.GateDir, "go-clean.log")
	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("go clean ran while the build was registered")
	}
	release()

	select {
	case out := <-done:
		if !strings.Contains(out, "done") {
			t.Errorf("output = %q, want done", out)
		}
	case <-time.After(gateWait):
		t.Fatal("go-clean-cache never ended after the build ended")
	}
	if b, err := os.ReadFile(logPath); err != nil || !strings.Contains(string(b), "clean -cache") {
		t.Errorf("go-clean.log = %q, %v; want a clean -cache", b, err)
	}
}
