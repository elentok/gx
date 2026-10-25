package brief

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type shellTool struct{ script string }

func (shellTool) Name() string { return "shell" }
func (s shellTool) Plan(dir string, args []string) (string, []string, string, error) {
	return "sh", []string{"-c", s.script}, "", nil
}
func (shellTool) Parse(out []byte, exit int) Outcome  { return Outcome{Units: 1, Summary: "1 thing"} }
func (shellTool) RerunArgs([]string, []Unit) []string { return nil }

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
