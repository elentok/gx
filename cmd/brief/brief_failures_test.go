package brief

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/testutil"
)

func TestBrief_FailuresListsEveryFailureOfTheLatestFailedRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	testutil.WriteFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	var src strings.Builder
	src.WriteString("package tiny\n\nimport \"testing\"\n\n")
	const n = 7 // more than the run report shows with text
	for i := range n {
		fmt.Fprintf(&src, "func TestBad%d(t *testing.T) { t.Errorf(\"cause %d\") }\n", i, i)
	}
	testutil.WriteFile(t, dir, "a_test.go", src.String())

	runOut, code := runBriefCmd(t, dir, "go-test", "./...")
	if code != 1 || !strings.Contains(runOut, "more failures are listed by name only") {
		t.Fatalf("run exit = %d, out:\n%s", code, runOut)
	}

	out, code := runBriefCmd(t, dir, "failures")
	if code != 1 {
		t.Fatalf("failures exit = %d, out:\n%s", code, out)
	}
	if !strings.HasPrefix(out, fmt.Sprintf("%d failed", n)) || strings.Contains(out, "listed by name only") {
		t.Errorf("want all %d failures with text, got:\n%s", n, out)
	}
	for i := range n {
		if want := fmt.Sprintf("cause %d", i); !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if lastLine(out) != lastLine(runOut) {
		t.Errorf("last line = %q, want the run's log %q", lastLine(out), lastLine(runOut))
	}

	again, code := runBriefCmd(t, dir, "failures", lastLine(runOut))
	if code != 1 || again != out {
		t.Errorf("failures <log> = %d, want the same report:\n%s", code, again)
	}
}

func TestBrief_FailuresWithATestNamePrintsOneExcerptAndTheRerunLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	testutil.WriteFile(t, dir, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	testutil.WriteFile(t, dir, "a_test.go", "package tiny\n\nimport \"testing\"\n\n"+
		"func TestOne(t *testing.T) { t.Errorf(\"cause one\") }\n"+
		"func TestTwo(t *testing.T) { t.Errorf(\"cause two\") }\n")
	runOut, _ := runBriefCmd(t, dir, "go-test", "./...")
	log := lastLine(runOut)

	out, code := runBriefCmd(t, dir, "failures", log, "TestTwo")
	if code != 1 || !strings.Contains(out, "cause two") || strings.Contains(out, "cause one") {
		t.Fatalf("exit = %d, want only TestTwo:\n%s", code, out)
	}
	if !strings.Contains(out, "re-run only the failing tests: gx brief go-test") || !strings.Contains(out, "TestTwo") {
		t.Errorf("no rerun line:\n%s", out)
	}

	out, code = runBriefCmd(t, dir, "failures", log, "TestNope")
	if code != 2 || !strings.Contains(out, "TestNope") {
		t.Errorf("unknown test: exit = %d, out:\n%s", code, out)
	}
}

func TestBrief_FailuresWithoutAFailedRunCannotRun(t *testing.T) {
	t.Parallel()
	out, code := runBriefCmd(t, t.TempDir(), "failures")
	if code != 2 || !strings.Contains(out, "no failed brief run") {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
}

func TestBrief_FailuresReadsACheckLogByTestName(t *testing.T) {
	t.Parallel()
	log, err := filepath.Abs("../../brief/testdata/check-test.log")
	if err != nil {
		t.Fatal(err)
	}
	out, code := runBriefCmd(t, t.TempDir(), "failures", log)
	if code != 1 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	for _, want := range []string{
		"TestHealthProcs_FirstCallIsReadingAndWaitGetsTheList", "expected 63.69 to be less than 40",
		"117. Default harness setting", "18. Joined card",
		"pause.e2e.ts.3. Pause reasons and recovery took 152.60s, over the 45s limit",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

// indexOf writes a brief index whose entries point at fixture logs and
// returns the directory to run in.
func indexOf(t *testing.T, entries ...brief.IndexEntry) string {
	t.Helper()
	env := cmdtest.EnvFor(t)
	data := filepath.Join(cmdtest.EnvValue(env, "XDG_DATA_HOME"), "gx")
	var lines []any
	for _, e := range entries {
		abs, err := filepath.Abs(filepath.Join("../../brief/testdata", e.Log))
		if err != nil {
			t.Fatal(err)
		}
		e.Log = abs
		lines = append(lines, e)
	}
	writeLines(t, filepath.Join(data, "brief", "index.jsonl"), lines...)
	return testutil.TempRepo(t)
}

func TestBrief_FailuresByFileCountsFindingsPerFileSorted(t *testing.T) {
	t.Parallel()
	repo := indexOf(t, brief.IndexEntry{Tool: "biome", Exit: 1, Log: "biome-byfile.txt"})
	out, code := runBriefCmd(t, repo, "failures", "--by-file")
	if code != 1 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	want := "3 src/c.ts\n2 src/a.ts\n1 src/b.ts\n"
	if !strings.Contains(out, want) || strings.Contains(out, "Avoid any") {
		t.Errorf("want %q and no finding text, got:\n%s", want, out)
	}
}

func TestBrief_FailuresRunsListsFailingE2eFilesPerRun(t *testing.T) {
	t.Parallel()
	repo := indexOf(t,
		brief.IndexEntry{Tool: "playwright", Exit: 1, Time: "2026-01-01T00:00:01Z", Log: "pw-run1.log"},
		brief.IndexEntry{Tool: "go-test", Exit: 1, Time: "2026-01-01T00:00:02Z", Log: "check-test.log"},
		brief.IndexEntry{Tool: "playwright", Exit: 1, Time: "2026-01-01T00:00:03Z", Log: "pw-run2.log"},
		brief.IndexEntry{Tool: "playwright", Exit: 0, Time: "2026-01-01T00:00:04Z", Log: "pw-run3.log"},
	)
	out, code := runBriefCmd(t, repo, "failures", "--runs", "2")
	if code != 0 {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
	want := "2026-01-01T00:00:03Z FAIL pause.e2e.ts\n2026-01-01T00:00:04Z PASS\n"
	if out != want {
		t.Errorf("want:\n%s\ngot:\n%s", want, out)
	}
	out, _ = runBriefCmd(t, repo, "failures", "--runs")
	if !strings.Contains(out, "2026-01-01T00:00:01Z FAIL disk.e2e.ts epics.e2e.ts\n") || strings.Contains(out, "go-test") {
		t.Errorf("default N should list all three playwright runs:\n%s", out)
	}
}

func TestBrief_FailuresSaysSoWhenALogFailedButNothingParsed(t *testing.T) {
	t.Parallel()
	log, err := filepath.Abs("../../brief/testdata/check-failed-unparsed.log")
	if err != nil {
		t.Fatal(err)
	}
	out, code := runBriefCmd(t, t.TempDir(), "failures", log)
	if code != 1 || !strings.Contains(out, "failed but no parsed failures") || !strings.Contains(out, "exit status 1") || strings.Contains(out, "no failures in") {
		t.Fatalf("exit = %d, out:\n%s", code, out)
	}
}
