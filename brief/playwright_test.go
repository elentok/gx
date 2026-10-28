package brief

import (
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
	want := []string{"test", "--reporter=line", "-c", playwrightConfig, "epics.e2e.ts", "-g", "^2\\."}
	if !slices.Equal(inv.Argv, want) {
		t.Errorf("argv = %v, want %v", inv.Argv, want)
	}
	if !strings.Contains(inv.Note, "replaced") {
		t.Errorf("note = %q, want a replaced-reporter note", inv.Note)
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
	want := []string{"test", "--reporter=line", "-c", playwrightConfig, "disk.e2e.ts", "--workers", "2", "-g", `› (69|70)\. `}
	if !slices.Equal(inv.Argv, want) {
		t.Errorf("argv = %v, want %v", inv.Argv, want)
	}
	// Playwright matches -g against "[project] › file:line:col › title", so a
	// "^" anchor on the bare title never matches; the pattern must find a flow.
	re := regexp.MustCompile(want[len(want)-1])
	if !re.MatchString("[chromium] › disk.e2e.ts:9:1 › 69. Disk banner") || re.MatchString("[chromium] › disk.e2e.ts:9:1 › 169. Other") {
		t.Errorf("pattern %q must match flow 69 and not 169", want[len(want)-1])
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

func TestPlaywrightPlan_RefusesADirWithoutTheConfig(t *testing.T) {
	dir := playwrightDir(t, false)
	_, err := Playwright{}.Plan(nil, dir, nil)
	if err == nil || !strings.Contains(err.Error(), "-C webui") {
		t.Errorf("err = %v, want advice to use -C webui", err)
	}
}
