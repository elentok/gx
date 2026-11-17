package brief

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// reportTool plans a run that writes report to Invocation.ReportFile.
type reportTool struct {
	shellTool
	report string
}

func (r reportTool) Plan(_ []string, _ string, _ []string) (Invocation, error) {
	return Invocation{Bin: "sh", Argv: []string{"-c", r.script}, Env: nil, ReportFile: r.report}, nil
}
func (reportTool) ReadReport(string) map[string]int64 { return nil }

func TestKeepReport_ByExitAndEnv(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		exit string
		keep bool
	}{
		{"failing with env keeps", []string{"GX_BRIEF_KEEP_REPORT=1"}, "1", true},
		{"passing with env keeps nothing", []string{"GX_BRIEF_KEEP_REPORT=1"}, "0", false},
		{"failing without env keeps nothing", nil, "1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := filepath.Join(t.TempDir(), "report.json")
			tool := reportTool{
				shellTool: shellTool{name: "playwright", script: `echo '{"ok":1}' > '` + report + `'; exit ` + tc.exit},
				report:    report,
			}
			data := t.TempDir()
			Run(tool, Options{Dir: t.TempDir(), DataDir: data, Out: &bytes.Buffer{}, Env: tc.env})

			if _, err := os.Stat(report); !os.IsNotExist(err) {
				t.Errorf("temp report still exists: %v", err)
			}
			kept, _ := filepath.Glob(filepath.Join(data, "brief", "*-playwright.json"))
			if !tc.keep {
				if len(kept) != 0 {
					t.Fatalf("kept %v, want none", kept)
				}
				return
			}
			logs, _ := filepath.Glob(filepath.Join(data, "brief", "*-playwright.log"))
			if len(logs) != 1 || len(kept) != 1 {
				t.Fatalf("logs %v, reports %v", logs, kept)
			}
			if want := logs[0][:len(logs[0])-len(".log")] + ".json"; kept[0] != want {
				t.Errorf("report at %s, want %s", kept[0], want)
			}
			if got, _ := os.ReadFile(kept[0]); string(got) != "{\"ok\":1}\n" {
				t.Errorf("report content %q", got)
			}
		})
	}
}

func TestSweep_RemovesAgedReports(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	old := now.Add(-Retention - time.Hour)
	for name, mod := range map[string]time.Time{"old.log": old, "old.json": old, "new.log": now, "new.json": now} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	sweep(dir, now)
	for name, want := range map[string]bool{"old.log": false, "old.json": false, "new.log": true, "new.json": true} {
		_, err := os.Stat(filepath.Join(dir, name))
		if (err == nil) != want {
			t.Errorf("%s exists=%v, want %v", name, err == nil, want)
		}
	}
}
