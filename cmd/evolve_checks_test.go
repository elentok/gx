package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/check"
)

var claimBase = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// checksFixture adds a verify ticket 09 and claim times to the inputs fixture
// and points the env at a temp data dir.
func checksFixture(t *testing.T) (*evolveFixture, string) {
	t.Helper()
	f := inputsFixture(t)
	body := "---\nid: \"09\"\nstatus: done\ntype: verify\nclaimed_at: " + claimBase.Format(time.RFC3339) + "\n---\nBody.\n"
	if err := os.WriteFile(filepath.Join(f.epic, "issues", "09-verify.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	f.env.dataDir = t.TempDir()
	return f, f.env.dataDir
}

func writeJSONL[T any](t *testing.T, path string, lines []T) {
	t.Helper()
	var buf bytes.Buffer
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fullRun(id string, at time.Time, branch, tree string, goMS int64) check.Record {
	return check.Record{Run: id, Time: at.Format(time.RFC3339), Scope: "full", Branch: branch, TreeID: tree,
		Kinds: []check.KindRecord{{Kind: "test", Status: check.StatusPass, DurationMS: goMS}}}
}

func inputsTable(t *testing.T, f *evolveFixture) InputsTable {
	t.Helper()
	var out bytes.Buffer
	if err := runEvolveInputs(f.env, &out, "proj:epic", true); err != nil {
		t.Fatal(err)
	}
	var table InputsTable
	if err := json.Unmarshal(out.Bytes(), &table); err != nil {
		t.Fatal(err)
	}
	return table
}

// baselineAndVerify writes 5 baseline runs of 100 s and a verify run of verifyMS.
func baselineAndVerify(t *testing.T, data string, verifyMS int64) {
	t.Helper()
	var recs []check.Record
	for i := range 5 {
		recs = append(recs, fullRun(fmt.Sprintf("b%d", i), claimBase.Add(-time.Duration(10-i)*time.Hour), "main", "t0", 100_000))
	}
	recs = append(recs, fullRun("v1", claimBase.Add(time.Hour), "ralph-loop/epic-item-09", "t1", verifyMS))
	writeJSONL(t, filepath.Join(data, "check", "proj.jsonl"), recs)
}

func TestEvolveInputs_SlowerRow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		verifyMS int64
		want     int
	}{
		{"40% and 40s slower", 140_000, 1},
		{"10% slower", 110_000, 0},
		{"over 20% but under 30s", 125_000, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, data := checksFixture(t)
			baselineAndVerify(t, data, tc.verifyMS)
			table := inputsTable(t, f)
			if len(table.Slower) != tc.want {
				t.Fatalf("slower = %+v, want %d rows", table.Slower, tc.want)
			}
			if tc.want == 1 {
				s := table.Slower[0]
				if s.Kind != "test" || s.BaselineMS != 100_000 || s.DurationMS != 140_000 || s.Run != "v1" {
					t.Errorf("row = %+v", s)
				}
			}
		})
	}
}

func TestEvolveInputs_SlowerNeedsFiveBaselineRuns(t *testing.T) {
	t.Parallel()
	f, data := checksFixture(t)
	recs := []check.Record{
		fullRun("b0", claimBase.Add(-time.Hour), "main", "t0", 100_000),
		fullRun("v1", claimBase.Add(time.Hour), "ralph-loop/epic-item-09", "t1", 200_000),
	}
	writeJSONL(t, filepath.Join(data, "check", "proj.jsonl"), recs)
	table := inputsTable(t, f)
	if len(table.Slower) != 0 || table.SlowerBaselineRuns != 1 {
		t.Errorf("slower = %+v, baseline runs = %d", table.Slower, table.SlowerBaselineRuns)
	}
}

func TestEvolveInputs_FlakeRow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		passTree string
		want     int
	}{
		{"same tree", "t1", 1},
		{"different tree", "t2", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, data := checksFixture(t)
			failing := check.Record{Run: "r1", Time: claimBase.Format(time.RFC3339), Scope: "changed",
				Branch: "ralph-loop/epic-item-02", TreeID: "t1",
				Kinds: []check.KindRecord{{Kind: "test", Status: check.StatusFail, Failed: []string{"pkg.TestX"}}}}
			writeJSONL(t, filepath.Join(data, "check", "proj.jsonl"), []check.Record{failing})
			writeJSONL(t, filepath.Join(data, "brief", "index.jsonl"), []brief.IndexEntry{{
				Time: claimBase.Add(time.Minute).Format(time.RFC3339), TreeID: tc.passTree,
				Tests: []string{"pkg.TestX"}, Log: "/logs/pass.log"}})
			table := inputsTable(t, f)
			if len(table.Flakes) != tc.want {
				t.Fatalf("flakes = %+v, want %d", table.Flakes, tc.want)
			}
			if tc.want == 1 {
				got := table.Flakes[0]
				if got.Test != "pkg.TestX" || got.FailingRun != "r1" || got.PassingRun != "/logs/pass.log" || got.TreeID != "t1" {
					t.Errorf("flake = %+v", got)
				}
			}
		})
	}
}

func TestEvolveInputs_FlakeSkipsRecordWithNoTree(t *testing.T) {
	t.Parallel()
	f, data := checksFixture(t)
	failing := check.Record{Run: "r1", Time: claimBase.Format(time.RFC3339), Scope: "changed",
		Branch: "ralph-loop/epic-item-02", TreeError: "not a git repository",
		Kinds: []check.KindRecord{{Kind: "test", Status: check.StatusFail, Failed: []string{"pkg.TestX"}}}}
	passing := fullRun("r2", claimBase.Add(time.Minute), "ralph-loop/epic-item-02", "", 1)
	writeJSONL(t, filepath.Join(data, "check", "proj.jsonl"), []check.Record{failing, passing})
	table := inputsTable(t, f)
	if len(table.Flakes) != 0 || table.FlakesNoTree != 1 {
		t.Errorf("flakes = %+v, no tree = %d", table.Flakes, table.FlakesNoTree)
	}
	var out bytes.Buffer
	if err := printEpicChecks(&out, table); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "skipped 1 records with no tree") {
		t.Errorf("output = %q", out.String())
	}
}

func TestEvolveInputs_NoRecordNoRows(t *testing.T) {
	t.Parallel()
	f, _ := checksFixture(t)
	table := inputsTable(t, f)
	if len(table.Slower) != 0 || len(table.Flakes) != 0 {
		t.Errorf("rows = %+v %+v", table.Slower, table.Flakes)
	}
}

func TestEvolveInputs_TextShowsCheckRows(t *testing.T) {
	t.Parallel()
	f, data := checksFixture(t)
	baselineAndVerify(t, data, 140_000)
	var out bytes.Buffer
	if err := runEvolveInputs(f.env, &out, "proj:epic", false); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("slower: test 140s vs baseline 100s")) {
		t.Errorf("output lacks slower line:\n%s", out.String())
	}
}
