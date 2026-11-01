package brief

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/gocachegate"
)

type shellTool struct{ script, name string }

func (s shellTool) Name() string {
	if s.name != "" {
		return s.name
	}
	return "shell"
}
func (s shellTool) Plan(_ []string, dir string, args []string) (Invocation, error) {
	return Invocation{Bin: "sh", Argv: []string{"-c", s.script}}, nil
}
func (shellTool) Parse(out []byte, exit int) Outcome  { return Outcome{Units: 1, Summary: "1 thing"} }
func (shellTool) RerunArgs([]string, []Unit) []string { return nil }

func TestCreateLog_SameStemTwiceGetsDistinctFiles(t *testing.T) {
	dir := t.TempDir()
	first, firstPath, err := createLog(dir, "20260101T000000.000000000-go-test")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, secondPath, err := createLog(dir, "20260101T000000.000000000-go-test")
	if err != nil {
		t.Fatalf("second log with the same stem: %v", err)
	}
	defer second.Close()
	if firstPath == secondPath {
		t.Fatalf("both logs share %s", firstPath)
	}
	if !strings.HasSuffix(secondPath, ".log") {
		t.Fatalf("second log %s lost its .log suffix", secondPath)
	}
}

// gateHeld reports whether a cleaner's exclusive wait runs into its cap, i.e.
// something holds the gate shared.
func gateHeld(t *testing.T, stateDir string) bool {
	t.Helper()
	d := gocachegate.Deps{Cap: 150 * time.Millisecond, Scan: func() (bool, error) { return false, nil }, Logf: func(string, ...any) {}}
	release, capPassed, err := d.WaitExclusive(context.Background(), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	release()
	return capPassed
}

func TestRun_GoToolsHoldCacheGateWhileRunning(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stateDir bool
		want     bool
	}{
		{"go-build", true, true},
		{"go-vet", true, true},
		{"go-test", true, true},
		{"vitest", true, false},
		{"go-test", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := filepath.Join(t.TempDir(), "state")
			started := filepath.Join(t.TempDir(), "started")
			release := filepath.Join(t.TempDir(), "release")
			tool := shellTool{name: tc.name, script: "touch " + started + "; while [ ! -e " + release + " ]; do sleep 0.02; done"}
			o := Options{Dir: t.TempDir(), DataDir: t.TempDir(), Out: &bytes.Buffer{}}
			if tc.stateDir {
				o.StateDir = stateDir
			}

			done := make(chan int)
			go func() { done <- Run(tool, o) }()
			deadline := time.Now().Add(10 * time.Second)
			for {
				if _, err := os.Stat(started); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("tool never started")
				}
				time.Sleep(20 * time.Millisecond)
			}
			if got := gateHeld(t, stateDir); got != tc.want {
				t.Errorf("gate held while running = %v, want %v", got, tc.want)
			}
			if err := os.WriteFile(release, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			if code := <-done; code != ExitPass {
				t.Fatalf("exit = %d, want %d", code, ExitPass)
			}
			if gateHeld(t, stateDir) {
				t.Error("gate still held after the tool ended")
			}
		})
	}
}

func TestRun_UnavailableGateDoesNotFailGoTool(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// A state dir under a regular file can be neither made nor opened.
	o := Options{Dir: t.TempDir(), DataDir: t.TempDir(), Out: &bytes.Buffer{}, StateDir: filepath.Join(blocker, "state")}
	if code := Run(shellTool{name: "go-build", script: "true"}, o); code != ExitPass {
		t.Fatalf("exit = %d, want %d", code, ExitPass)
	}
}

func TestRun_WritesLogWhileToolRuns(t *testing.T) {
	dataDir := t.TempDir()
	release := filepath.Join(t.TempDir(), "release")
	tool := shellTool{script: "echo started; while [ ! -e " + release + " ]; do sleep 0.02; done"}

	done := make(chan int)
	go func() {
		done <- Run(tool, Options{Dir: t.TempDir(), DataDir: dataDir, Out: &bytes.Buffer{}})
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		logs, _ := filepath.Glob(filepath.Join(dataDir, "brief", "*.log"))
		if len(logs) == 1 {
			if b, _ := os.ReadFile(logs[0]); strings.Contains(string(b), "started") {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("log has no tool output while the tool is still running")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := <-done; code != ExitPass {
		t.Fatalf("exit = %d, want %d", code, ExitPass)
	}
}
