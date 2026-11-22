package brief

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/check"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/testutil"
)

func writeLines(t *testing.T, path string, rows ...any) {
	t.Helper()
	var sb strings.Builder
	for _, r := range rows {
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBrief_HealthPrintsHeadlineFlipAndPercentiles(t *testing.T) {
	t.Parallel()
	env := cmdtest.EnvFor(t)
	data := filepath.Join(cmdtest.EnvValue(env, "XDG_DATA_HOME"), "gx")
	repo := testutil.TempRepo(t)
	project := check.ProjectForDirs(config.DirsFrom(env), repo)

	rec := func(run string, status check.Status, failed ...string) check.Record {
		return check.Record{Run: run, Scope: "full", Link: check.LinkTrunk, TreeID: "t1",
			Kinds: []check.KindRecord{{Kind: "test", Status: status, Failed: failed}}}
	}
	writeLines(t, filepath.Join(data, "check", project+".jsonl"),
		rec("r1", check.StatusFail, "a.TestFlip"), rec("r2", check.StatusPass))
	durations := map[string]int64{"a.TestFlip": 40}
	var idx []any
	idx = append(idx, brief.IndexEntry{Tool: "go-test", Time: "2026-01-01T00:00:00Z", CheckRun: "r1", TreeID: "t1",
		Tests: []string{"a.TestFlip"}, Failed: []string{"a.TestFlip"}, TestDurations: durations})
	for range 4 {
		idx = append(idx, brief.IndexEntry{Tool: "go-test", Time: "2026-01-02T00:00:00Z", CheckRun: "r2", TreeID: "t1",
			Tests: []string{"a.TestFlip"}, TestDurations: durations})
	}
	writeLines(t, filepath.Join(data, "brief", "index.jsonl"), idx...)

	for _, bad := range [][]string{{"--nope"}, {"--limit"}, {"--limit", "0"}, {"--limit", "x"}, {"extra"}} {
		out, code := runBriefCmd(t, repo, append([]string{"health"}, bad...)...)
		if code != brief.ExitCantRun || !strings.HasPrefix(out, "brief: ") {
			t.Errorf("health %v: exit %d, out %q; want exit 2 and one brief: line", bad, code, out)
		}
	}
	if _, code := runBriefCmd(t, t.TempDir(), "-C", repo, "health", "--limit", "5", "--all-runs"); code != 0 {
		t.Errorf("-C <dir> health with flags: exit %d", code)
	}
	if _, code := runBriefCmd(t, repo, "failures", "--nope"); code != brief.ExitCantRun {
		t.Errorf("failures --nope: exit %d, want 2", code)
	}

	out, code := runBriefCmd(t, repo, "health")
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, want := range []string{
		"0 of 1 first attempts passed (0%)",
		"streak: 1",
		"1 trees  2026-01-02T00:00:00Z  a.TestFlip",
		"p50 40ms  p95 40ms  (5 samples)  a.TestFlip",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
