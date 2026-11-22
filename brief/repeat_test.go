package brief

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// failOn builds a script that fails when the call counter hits one of nums.
func failOn(t *testing.T, nums ...string) string {
	t.Helper()
	calls := filepath.Join(t.TempDir(), "calls")
	script := `echo x >> '` + calls + `'; n=$(wc -l < '` + calls + `' | tr -d ' '); echo run-$n; case $n in `
	for _, n := range nums {
		script += n + ") exit 1;; "
	}
	return script + "esac"
}

func TestRepeat_SummaryLogAndExit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		script   string
		wantExit int
		wantLine string
		wantLogs int
	}{
		{"all pass", "true", ExitPass, "3 runs: 3 passed, 0 failed", 0},
		{"some fail", failOn(t, "2", "3"), ExitFail, "3 runs: 1 passed, 2 failed", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			o := Options{Dir: t.TempDir(), DataDir: t.TempDir(), Out: &out}
			if got := Repeat(shellTool{script: tc.script}, o, 3, 1); got != tc.wantExit {
				t.Fatalf("exit = %d, want %d\n%s", got, tc.wantExit, out.String())
			}
			lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
			if lines[0] != tc.wantLine {
				t.Errorf("first line = %q, want %q", lines[0], tc.wantLine)
			}
			if tc.wantLogs == 0 {
				if len(lines) != 1 {
					t.Errorf("a clean run prints one line, got:\n%s", out.String())
				}
				return
			}
			last := lines[len(lines)-1]
			data, err := os.ReadFile(last)
			if err != nil {
				t.Fatalf("last line %q is not a log path: %v", last, err)
			}
			if !strings.Contains(string(data), "run-2") {
				t.Errorf("last line must be the first failing run's log (run 2), got:\n%s", data)
			}
			var failing int
			entries, _ := ReadIndex(o.DataDir)
			for _, e := range entries {
				if e.Exit == ExitFail {
					failing++
					if _, err := os.Stat(e.Log); err != nil {
						t.Errorf("failing run lost its log: %v", err)
					}
				}
			}
			if failing != tc.wantLogs {
				t.Errorf("failing runs in index = %d, want %d", failing, tc.wantLogs)
			}
		})
	}
}

func TestRepeat_ParallelRunsEveryRun(t *testing.T) {
	var out bytes.Buffer
	o := Options{Dir: t.TempDir(), DataDir: t.TempDir(), Out: &out}
	if got := Repeat(shellTool{script: "true"}, o, 6, 3); got != ExitPass {
		t.Fatalf("exit = %d\n%s", got, out.String())
	}
	if !strings.HasPrefix(out.String(), "6 runs: 6 passed, 0 failed") {
		t.Errorf("got %q", out.String())
	}
	if entries, _ := ReadIndex(o.DataDir); len(entries) != 6 {
		t.Errorf("index lines = %d, want 6", len(entries))
	}
}

func TestRepeat_RunThatCannotExecuteExits2(t *testing.T) {
	var out bytes.Buffer
	o := Options{Dir: filepath.Join(t.TempDir(), "missing"), DataDir: t.TempDir(), Out: &out}
	if got := Repeat(shellTool{script: "true"}, o, 2, 1); got != ExitCantRun {
		t.Fatalf("exit = %d, want %d\n%s", got, ExitCantRun, out.String())
	}
	if !strings.Contains(out.String(), "2 runs: 0 passed, 0 failed, 2 could not run") {
		t.Errorf("got %q", out.String())
	}
}
