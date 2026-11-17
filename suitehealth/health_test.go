package suitehealth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/check"
)

const proj = "p"

// writeJSONL writes one JSON value per line under dir.
func writeJSONL[T any](t *testing.T, path string, rows []T) {
	t.Helper()
	var b bytes.Buffer
	for _, r := range rows {
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixture writes the records and index lines and returns the data dir.
func fixture(t *testing.T, recs []check.Record, idx []brief.IndexEntry) string {
	t.Helper()
	dir := t.TempDir()
	writeJSONL(t, filepath.Join(dir, "check", proj+".jsonl"), recs)
	writeJSONL(t, filepath.Join(dir, "brief", "index.jsonl"), idx)
	return dir
}

func fullRun(run, tree string, kinds ...check.KindRecord) check.Record {
	return check.Record{Run: run, Scope: "full", Link: check.LinkTrunk, TreeID: tree, Kinds: kinds}
}

func pass() check.KindRecord { return check.KindRecord{Kind: "test", Status: check.StatusPass} }

func fail(names ...string) check.KindRecord {
	return check.KindRecord{Kind: "test", Status: check.StatusFail, Failed: names}
}

func health(t *testing.T, dir string, o Options) (string, int) {
	t.Helper()
	o.DataDir, o.Project = dir, proj
	var out bytes.Buffer
	code := Health(o, &out)
	return out.String(), code
}

func wantLine(t *testing.T, out, want string) {
	t.Helper()
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}
}

func TestSuiteHealth_RerunOfFailedTreeCountsAsFailure(t *testing.T) {
	t.Parallel()
	dir := fixture(t, []check.Record{
		fullRun("r1", "t1", fail("a.TestX")),
		fullRun("r2", "t1", pass()),
	}, nil)
	out, code := health(t, dir, Options{})
	wantLine(t, out, "0 of 1 first attempts passed (0%)")
	wantLine(t, out, "streak: 1")
	if code != 0 {
		t.Errorf("exit %d", code)
	}
}

func TestSuiteHealth_SameTreeTwicePassingCountsOnce(t *testing.T) {
	t.Parallel()
	dir := fixture(t, []check.Record{fullRun("r1", "t1", pass()), fullRun("r2", "t1", pass())}, nil)
	out, _ := health(t, dir, Options{})
	wantLine(t, out, "1 of 1 first attempts passed (100%)")
}

func TestSuiteHealth_OnlyFullTrunkRunsCount(t *testing.T) {
	t.Parallel()
	changed := fullRun("r1", "t1", fail("a.TestX"))
	changed.Scope = "changed"
	filtered := fullRun("r2", "t2", fail("a.TestX"))
	filtered.KindFilter = "test"
	ticket := fullRun("r3", "t3", fail("a.TestX"))
	ticket.Link = check.LinkTicket
	dir := fixture(t, []check.Record{changed, filtered, ticket}, nil)
	out, _ := health(t, dir, Options{})
	wantLine(t, out, "0 of 0 first attempts passed")
}

func TestSuiteHealth_GuardOnlyFailureCountsAsPassed(t *testing.T) {
	t.Parallel()
	dir := fixture(t, []check.Record{fullRun("r1", "t1", fail("github.com/elentok/gx/sizeguard.TestLimits"))}, nil)
	out, _ := health(t, dir, Options{})
	wantLine(t, out, "1 of 1 first attempts passed")
}

func TestSuiteHealth_SlowOnlyFailureCountsAsPassed(t *testing.T) {
	t.Parallel()
	k := fail("a.TestSlow")
	k.Slow = []string{"a.TestSlow"}
	dir := fixture(t, []check.Record{fullRun("r1", "t1", k)}, nil)
	out, code := health(t, dir, Options{})
	wantLine(t, out, "1 of 1 first attempts passed")
	if code != 0 {
		t.Errorf("exit %d", code)
	}
}

func TestSuiteHealth_FailingKindWithoutNamesCountsAsFailed(t *testing.T) {
	t.Parallel()
	dir := fixture(t, []check.Record{fullRun("r1", "t1", fail())}, nil)
	out, _ := health(t, dir, Options{})
	wantLine(t, out, "0 of 1 first attempts passed")
}

func TestSuiteHealth_StreakStopsAtNewestFailure(t *testing.T) {
	t.Parallel()
	dir := fixture(t, []check.Record{
		fullRun("r1", "t1", pass()),
		fullRun("r2", "t2", fail("a.TestX")),
		fullRun("r3", "t3", pass()),
		fullRun("r4", "t4", pass()),
	}, nil)
	out, _ := health(t, dir, Options{})
	wantLine(t, out, "3 of 4 first attempts passed (75%)")
	wantLine(t, out, "streak: 2")
}

func goLine(run, tree, time string, tests []string, failed ...string) brief.IndexEntry {
	return brief.IndexEntry{Tool: "go-test", Time: time, CheckRun: run, TreeID: tree, Tests: tests, Failed: failed}
}

func TestSuiteHealth_FlipListNeedsFailThenPassOnOneTree(t *testing.T) {
	t.Parallel()
	both := []string{"a.TestFlip", "a.TestOnlyFails"}
	dir := fixture(t,
		[]check.Record{fullRun("r1", "t1", pass()), fullRun("r2", "t1", pass())},
		[]brief.IndexEntry{
			goLine("r1", "t1", "2026-01-01T00:00:00Z", both, "a.TestFlip", "a.TestOnlyFails"),
			goLine("r2", "t1", "2026-01-02T00:00:00Z", []string{"a.TestFlip"}),
		})
	out, _ := health(t, dir, Options{})
	wantLine(t, out, "1 trees  2026-01-02T00:00:00Z  a.TestFlip")
	if strings.Contains(out, "a.TestOnlyFails") {
		t.Errorf("a test that only fails is in the flip list:\n%s", out)
	}
}

func TestSuiteHealth_AllRunsReadsLinesWithoutFullTrunkRun(t *testing.T) {
	t.Parallel()
	idx := []brief.IndexEntry{
		goLine("", "t9", "2026-01-01T00:00:00Z", []string{"a.TestFlip"}, "a.TestFlip"),
		goLine("", "t9", "2026-01-02T00:00:00Z", []string{"a.TestFlip"}),
	}
	dir := fixture(t, nil, idx)
	out, _ := health(t, dir, Options{})
	if strings.Contains(out, "a.TestFlip") {
		t.Errorf("default read used a line of no full trunk run:\n%s", out)
	}
	out, _ = health(t, dir, Options{AllRuns: true})
	wantLine(t, out, "a.TestFlip")
}

func TestSuiteHealth_OldVitestNameAndNewNameAreOneTest(t *testing.T) {
	t.Parallel()
	old := brief.IndexEntry{Tool: "vitest", Dir: "/w/one/webui", Time: "2026-01-01T00:00:00Z", CheckRun: "r1", TreeID: "t1",
		Tests: []string{"/w/one/webui/src/a.test.ts > x"}, Failed: []string{"/w/one/webui/src/a.test.ts > x"}}
	fresh := brief.IndexEntry{Tool: "vitest", Dir: "/w/two/webui", Time: "2026-01-02T00:00:00Z", CheckRun: "r2", TreeID: "t1",
		Tests: []string{"src/a.test.ts > x"}}
	dir := fixture(t, []check.Record{fullRun("r1", "t1", pass()), fullRun("r2", "t1", pass())}, []brief.IndexEntry{old, fresh})
	out, _ := health(t, dir, Options{})
	wantLine(t, out, "1 trees  2026-01-02T00:00:00Z  src/a.test.ts > x")
}

func TestSuiteHealth_PlaywrightPassingNamesComeFromDurations(t *testing.T) {
	t.Parallel()
	first := brief.IndexEntry{Tool: "playwright", Time: "2026-01-01T00:00:00Z", CheckRun: "r1", TreeID: "t1",
		Failed: []string{"flow.e2e.ts > a"}, TestDurations: map[string]int64{"flow.e2e.ts > b": 5}}
	second := brief.IndexEntry{Tool: "playwright", Time: "2026-01-02T00:00:00Z", CheckRun: "r2", TreeID: "t1",
		TestDurations: map[string]int64{"flow.e2e.ts > a": 5}}
	dir := fixture(t, []check.Record{fullRun("r1", "t1", pass()), fullRun("r2", "t1", pass())}, []brief.IndexEntry{first, second})
	out, _ := health(t, dir, Options{})
	wantLine(t, out, "flow.e2e.ts > a")
}

func TestSuiteHealth_PercentileOfKnownList(t *testing.T) {
	t.Parallel()
	var ms []int64
	for i := int64(1); i <= 20; i++ {
		ms = append(ms, i*10)
	}
	if p50, p95 := Percentile(ms, 50), Percentile(ms, 95); p50 != 100 || p95 != 190 {
		t.Errorf("p50 %d, p95 %d", p50, p95)
	}
}

func TestSuiteHealth_DurationsNeedFiveSamples(t *testing.T) {
	t.Parallel()
	var idx []brief.IndexEntry
	for i := range 5 {
		d := map[string]int64{"a.TestFive": int64(10 * (i + 1))}
		if i < 4 {
			d["a.TestFour"] = 1000
		}
		idx = append(idx, brief.IndexEntry{Tool: "go-test", Time: fmt.Sprintf("2026-01-0%dT00:00:00Z", i+1),
			CheckRun: "r1", TreeID: "t1", TestDurations: d})
	}
	dir := fixture(t, []check.Record{fullRun("r1", "t1", pass())}, idx)
	out, _ := health(t, dir, Options{})
	wantLine(t, out, "p50 30ms  p95 50ms  (5 samples)  a.TestFive")
	if strings.Contains(out, "a.TestFour") {
		t.Errorf("a test with 4 samples is listed:\n%s", out)
	}
}

func TestSuiteHealth_ExitCodeIsZeroWhateverTheNumbers(t *testing.T) {
	t.Parallel()
	dir := fixture(t, []check.Record{fullRun("r1", "t1", fail("a.TestX"))}, nil)
	if _, code := health(t, dir, Options{}); code != 0 {
		t.Errorf("exit %d", code)
	}
}

func TestSuiteHealth_UnreadableDataExitsTwo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "brief", "index.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, code := health(t, dir, Options{}); code != ExitCantRun {
		t.Errorf("exit %d, want %d", code, ExitCantRun)
	}
}
