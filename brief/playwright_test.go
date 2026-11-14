package brief

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestPlaywrightParse_KeepsOnlyTheFirstErrorOfEachFailure(t *testing.T) {
	out := `Running 4 tests using 3 workers
[chromium] › epics.e2e.ts:12:5 › 2. Epic looks right
  1) [chromium] › epics.e2e.ts:12:5 › 2. Epic looks right ──────────

    Error: expect(locator).toBeVisible() failed

    Locator: getByText('x')
    Expected: visible

    Call log:
      - waiting for getByText('x')

      14 |   await a
    > 15 |   await expect(x).toBeVisible()
         |                   ^
        at /repo/webui/e2e/epics.e2e.ts:15:5

  1 failed
    [chromium] › epics.e2e.ts:12:5 › 2. Epic looks right
  3 passed (4.2s)
`
	got := Playwright{}.Parse([]byte(out), 1)
	if got.Units != 4 || got.Summary != "4 tests" {
		t.Errorf("units, summary = %d, %q, want 4, \"4 tests\"", got.Units, got.Summary)
	}
	if len(got.Failures) != 1 {
		t.Fatalf("failures = %+v, want one", got.Failures)
	}
	f := got.Failures[0]
	want := "Error: expect(locator).toBeVisible() failed\n\nLocator: getByText('x')\nExpected: visible"
	if f.Location != "epics.e2e.ts" || f.Name != "2. Epic looks right" || f.Text != want {
		t.Errorf("failure = %+v, want text %q", f, want)
	}
}

func TestPlaywrightParse_ListModeReturnsOneNumberedTitlePerFlow(t *testing.T) {
	out := `Listing tests:
  [chromium] › design.e2e.ts:12:5 › 5. Allow once
  [chromium] › design.e2e.ts:30:5 › 13. All-defaulted round
Total: 2 tests in 1 file
`
	got := Playwright{}.Parse([]byte(out), 0)
	want := []string{"5. Allow once", "13. All-defaulted round"}
	if !slices.Equal(got.Listing, want) {
		t.Errorf("listing = %q, want %q", got.Listing, want)
	}
}

// A failure whose block has no parsable heading is still named in the list
// under "N failed", so the record must keep it.
func TestPlaywrightParse_FailureListNamesFailuresWithoutAHeading(t *testing.T) {
	out := `Running 4 tests using 3 workers
  1) [chromium] › epics.e2e.ts:12:5 › 2. Epic looks right ──────────

    Error: boom

  3 failed
    [chromium] › epics.e2e.ts:12:5 › 2. Epic looks right
    [chromium] › pause.e2e.ts:30:1 › 7. Pause works
    [chromium] › queue.e2e.ts:8:3 › 9. Queue empties
  1 passed (4.2s)
`
	got := Playwright{}.Parse([]byte(out), 1)

	var names []string
	for _, f := range got.Failures {
		names = append(names, f.Location+"|"+f.Name)
	}
	want := []string{"epics.e2e.ts|2. Epic looks right", "pause.e2e.ts|7. Pause works", "queue.e2e.ts|9. Queue empties"}
	if !slices.Equal(names, want) {
		t.Errorf("failures = %q, want %q", names, want)
	}
	if got.Units != 4 {
		t.Errorf("units = %d, want 4", got.Units)
	}
}

func TestPlaywrightParse_NoTestsFoundIsEmpty(t *testing.T) {
	if got := (Playwright{}).Parse([]byte("Error: No tests found\n"), 1); got.Units != 0 || len(got.Failures) != 0 {
		t.Errorf("outcome = %+v, want empty", got)
	}
}

func playwrightDir(t *testing.T, config bool) string {
	t.Helper()
	orig := buildWeb
	t.Cleanup(func() { buildWeb = orig })
	root := t.TempDir()
	dir := filepath.Join(root, "webui")
	for _, p := range []string{filepath.Join(root, ".git"), filepath.Join(dir, "node_modules", ".bin"), filepath.Join(dir, "e2e")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{filepath.Join(dir, "node_modules", ".bin", "playwright")}
	if config {
		files = append(files, filepath.Join(dir, playwrightConfig))
	}
	for _, f := range files {
		if err := os.WriteFile(f, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPlaywrightPlan_AddsConfigAndLineReporter(t *testing.T) {
	dir := playwrightDir(t, true)
	os.MkdirAll(filepath.Join(dir, "dist"), 0o755)
	os.WriteFile(filepath.Join(dir, "dist", "index.html"), nil, 0o644)
	buildWeb = func([]string, string) error { t.Fatal("built although dist/index.html exists"); return nil }

	inv, err := Playwright{}.Plan(nil, dir, []string{"--reporter=list", "epics.e2e.ts", "-g", "^2\\."})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"test", "--reporter=line,json", "-c", playwrightConfig, "epics.e2e.ts", "-g", "^2\\."}
	if !slices.Equal(inv.Argv, want) {
		t.Errorf("argv = %v, want %v", inv.Argv, want)
	}
	if inv.ReportFile == "" || !slices.Contains(inv.Env, "PLAYWRIGHT_JSON_OUTPUT_NAME="+inv.ReportFile) {
		t.Errorf("report file %q, env %v: want the json reporter's output named", inv.ReportFile, inv.Env)
	}
	if !strings.Contains(inv.Note, "replaced") {
		t.Errorf("note = %q, want a replaced-reporter note", inv.Note)
	}
}

func TestPlaywrightPlan_HelpPrintsUsageInsteadOfRunningTheSuite(t *testing.T) {
	dir := playwrightDir(t, true)
	for _, flag := range []string{"--help", "-h"} {
		_, err := Playwright{}.Plan(nil, dir, []string{flag})
		if err == nil || !strings.Contains(err.Error(), "usage: gx brief -C webui playwright") {
			t.Errorf("%s: err = %v, want usage", flag, err)
		}
	}
}

func TestPlaywrightPlan_BareFlowNumbersBecomeOneAnchoredGrep(t *testing.T) {
	dir := playwrightDir(t, true)
	os.MkdirAll(filepath.Join(dir, "dist"), 0o755)
	os.WriteFile(filepath.Join(dir, "dist", "index.html"), nil, 0o644)

	inv, err := Playwright{}.Plan(nil, dir, []string{"disk.e2e.ts", "69", "70", "--workers", "2"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"test", "--reporter=line,json", "-c", playwrightConfig, "disk.e2e.ts", "--workers", "2", "-g", `(?:\.tsx?|›) (69|70)\. `}
	if !slices.Equal(inv.Argv, want) {
		t.Errorf("argv = %v, want %v", inv.Argv, want)
	}
	// Playwright matches -g against the project, file and title joined by
	// spaces (no ":line:col"), or by " › " in other versions.
	re := regexp.MustCompile(want[len(want)-1])
	for _, s := range []string{"chromium disk.e2e.ts 69. Disk banner", "[chromium] › disk.e2e.ts:9:1 › 69. Disk banner"} {
		if !re.MatchString(s) {
			t.Errorf("pattern %q must match %q", re, s)
		}
	}
	for _, s := range []string{"chromium disk.e2e.ts 169. Other", "chromium a.e2e.ts 157. x; 69. design y"} {
		if re.MatchString(s) {
			t.Errorf("pattern %q must not match %q", re, s)
		}
	}
}

func TestFlowNumbersToGrep(t *testing.T) {
	const grep = `(?:\.tsx?|›) (69)\. `
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr bool
	}{
		{"number after a boolean flag is a flow", []string{"--headed", "69"}, []string{"--headed", "-g", grep}, false},
		{"number after a value flag stays", []string{"--workers", "2"}, []string{"--workers", "2"}, false},
		{"no numbers leaves args alone", []string{"a.e2e.ts", "-g", "x"}, []string{"a.e2e.ts", "-g", "x"}, false},
		{"numbers mixed with -g", []string{"69", "-g", "x"}, nil, true},
		{"numbers mixed with --grep", []string{"--grep", "x", "69"}, nil, true},
		{"numbers mixed with --grep=", []string{"69", "--grep=x"}, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := flowNumbersToGrep(tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlaywrightPlan_BuildsWebOnlyWhenDistIsMissing(t *testing.T) {
	dir := playwrightDir(t, true)
	var builtAt string
	var builtEnv []string
	buildWeb = func(env []string, root string) error { builtAt, builtEnv = root, env; return nil }

	env := []string{"PATH=/fake/bin"}
	if _, err := (Playwright{}).Plan(env, dir, nil); err != nil {
		t.Fatal(err)
	}
	if builtAt != filepath.Dir(dir) {
		t.Errorf("built at %q, want the git root %q", builtAt, filepath.Dir(dir))
	}
	if !slices.Equal(builtEnv, env) {
		t.Errorf("built with env %v, want %v", builtEnv, env)
	}
}

func TestPlaywrightReadReport_KeysDurationsLikeFailureNames(t *testing.T) {
	report := `{"suites":[{"title":"epics.e2e.ts","file":"epics.e2e.ts","specs":[
	  {"title":"2. Epic looks right","file":"epics.e2e.ts","tests":[{"results":[{"duration":1200.4}]}]}],
	  "suites":[{"title":"queue","file":"epics.e2e.ts","specs":[
	  {"title":"3. Flaky","file":"epics.e2e.ts","tests":[{"results":[{"duration":100},{"duration":50}]}]}]}]}]}`
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Playwright{}.ReadReport(path)
	want := map[string]int64{
		"epics.e2e.ts.2. Epic looks right": 1200,
		"epics.e2e.ts.queue › 3. Flaky":    150,
	}
	if !maps.Equal(got, want) {
		t.Errorf("durations = %v, want %v", got, want)
	}
	if (Playwright{}).ReadReport(filepath.Join(t.TempDir(), "missing.json")) != nil {
		t.Error("a missing report must yield nil")
	}
}

func runReport(o Outcome, exit int) string {
	var buf bytes.Buffer
	report(Playwright{}, Options{Out: &buf}, o, nil, indexEntry{Exit: exit, Log: "LOG"})
	return buf.String()
}

func TestReport_PlaywrightPrintsTheSlowestFiveOnPassAndFail(t *testing.T) {
	d := map[string]int64{}
	for i := 1; i <= 7; i++ {
		d[fmt.Sprintf("a.e2e.ts.%d. T", i)] = int64(i) * 1000
	}
	pass := runReport(Outcome{Units: 7, Summary: "7 tests", TestDurations: d}, 0)
	want := "PASS playwright: 7 tests in 0s\n7.0s  a.e2e.ts.7. T\n6.0s  a.e2e.ts.6. T\n5.0s  a.e2e.ts.5. T\n4.0s  a.e2e.ts.4. T\n3.0s  a.e2e.ts.3. T\nLOG\n"
	if pass != want {
		t.Errorf("pass report:\n%s\nwant:\n%s", pass, want)
	}
	fail := runReport(Outcome{Failures: []Unit{{Location: "a.e2e.ts", Name: "7. T"}}, TestDurations: d}, 1)
	if !strings.Contains(fail, "7.0s  a.e2e.ts.7. T\n") || !strings.HasSuffix(fail, "LOG\n") {
		t.Errorf("fail report lacks the slowest tests:\n%s", fail)
	}
	few := runReport(Outcome{Units: 2, Summary: "2 tests", TestDurations: map[string]int64{"a.1": 500, "a.2": 1500}}, 0)
	if strings.Count(few, "s  a.") != 2 || !strings.Contains(few, "1.5s  a.2\n0.5s  a.1\n") {
		t.Errorf("fewer than five tests must all print:\n%s", few)
	}
	if none := runReport(Outcome{Units: 2, Summary: "2 tests"}, 0); strings.Contains(none, "s  ") {
		t.Errorf("no durations must print no lines:\n%s", none)
	}
}

func TestReport_PlaywrightPrintsMatchingSnapshotLinesOnFailureOnly(t *testing.T) {
	text := "Error: expect(locator).toBeVisible()\nLocator: getByRole('region', { name: 'Session failure' })\nExpected: visible"
	snapshot := "- main:\n  - button \"Dismiss\"\n  - region \"Session failure\"\n  - heading \"Other\""
	u := Unit{Location: "e2e/a.e2e.ts", Name: "1. T", Text: text}
	o := Outcome{Failures: []Unit{u}, Contexts: map[string]string{"a.e2e.ts.1. T": snapshot}}
	got := runReport(o, 1)
	if !strings.Contains(got, "    - region \"Session failure\"\n") || strings.Contains(got, "Other") || strings.Contains(got, "Dismiss") {
		t.Errorf("snapshot excerpt wrong:\n%s", got)
	}
	if pass := runReport(Outcome{Units: 1, Summary: "1 tests", Contexts: o.Contexts}, 0); strings.Contains(pass, "Session failure") {
		t.Errorf("a pass must print no snapshot:\n%s", pass)
	}
	o.Contexts = nil
	if got := runReport(o, 1); strings.Contains(got, "snapshot") {
		t.Errorf("no error-context.md must print no snapshot block:\n%s", got)
	}
}

func TestPlaywrightParse_FailureHeadingWithoutRule(t *testing.T) {
	out := "  1) [chromium] › e2e/a.e2e.ts:177:1 › 158. A very long title that fills the line \n\n    Error: boom\n\n  1 failed\n    [chromium] › e2e/a.e2e.ts:177:1 › 158. A very long title that fills the line\n"
	got := Playwright{}.Parse([]byte(out), 1)
	if len(got.Failures) != 1 || got.Failures[0].Text != "Error: boom" {
		t.Errorf("failures = %+v, want one with its error text", got.Failures)
	}
}

func TestPlaywrightReadContexts_ReadsTheAttachedErrorContext(t *testing.T) {
	dir := t.TempDir()
	ctx := filepath.Join(dir, "error-context.md")
	if err := os.WriteFile(ctx, []byte("- region \"X\""), 0o644); err != nil {
		t.Fatal(err)
	}
	report := fmt.Sprintf(`{"suites":[{"title":"a.e2e.ts","file":"a.e2e.ts","specs":[
	  {"title":"1. T","file":"a.e2e.ts","tests":[{"results":[{"duration":1,"attachments":[{"name":"error-context","path":%q}]}]}]},
	  {"title":"2. U","file":"a.e2e.ts","tests":[{"results":[{"duration":1,"attachments":[{"name":"error-context","path":%q}]}]}]}]}]}`,
		ctx, filepath.Join(dir, "missing.md"))
	path := filepath.Join(dir, "report.json")
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Playwright{}.ReadContexts(path)
	if want := map[string]string{"a.e2e.ts.1. T": "- region \"X\""}; !maps.Equal(got, want) {
		t.Errorf("contexts = %v, want %v", got, want)
	}
}

func TestPlaywrightPlan_RefusesADirWithoutTheConfig(t *testing.T) {
	dir := playwrightDir(t, false)
	_, err := Playwright{}.Plan(nil, dir, nil)
	if err == nil || !strings.Contains(err.Error(), "-C webui") {
		t.Errorf("err = %v, want advice to use -C webui", err)
	}
}
