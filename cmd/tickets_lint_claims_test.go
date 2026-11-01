package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

type lintFixture struct {
	t       *testing.T
	repo    string
	issues  string
	epicDir string
}

func newLintFixture(t *testing.T) *lintFixture {
	t.Helper()
	repo, project := setupShowProject(t)
	epicDir := filepath.Join(project, "epic")
	issues := filepath.Join(epicDir, "issues")
	if err := os.Remove(filepath.Join(issues, "06-hello.md")); err != nil {
		t.Fatal(err)
	}
	return &lintFixture{t: t, repo: repo, issues: issues, epicDir: epicDir}
}

// ticket writes a ticket; blockedBy is "" or a YAML list such as `["01"]`.
func (f *lintFixture) ticket(id, typ, blockedBy, body string) {
	f.t.Helper()
	fm := fmt.Sprintf("---\nid: %q\nstatus: open\ntype: %s\n", id, typ)
	if blockedBy != "" {
		fm += "blocked_by: " + blockedBy + "\n"
	}
	testutil.WriteFile(f.t, f.issues, id+"-t.md", fm+"---\n\n# T\n\n"+body)
}

func (f *lintFixture) file(rel, content string) {
	f.t.Helper()
	testutil.WriteFile(f.t, f.repo, rel, content)
}

// lint runs lint-claims and returns its output and exit code.
func (f *lintFixture) lint(extra ...string) (string, int) {
	f.t.Helper()
	out, err := runIn(f.t, f.repo, append([]string{"tickets", "lint-claims", "mine:epic"}, extra...)...)
	var ee *ExitError
	switch {
	case err == nil:
		return out, 0
	case errors.As(err, &ee):
		return out, ee.Code
	}
	f.t.Fatalf("lint-claims: %v", err)
	return "", -1
}

func wantLines(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output lacks %q:\n%s", w, out)
		}
	}
}

func wantNoLines(t *testing.T, out string, bad ...string) {
	t.Helper()
	for _, b := range bad {
		if strings.Contains(out, b) {
			t.Errorf("output has %q:\n%s", b, out)
		}
	}
}

func TestLintClaims_CommandsPathsPatternsFlows(t *testing.T) {
	f := newLintFixture(t)
	f.file("webui/e2e/CATALOG.md", "# Catalog\n\n## 5. Five\n\n## 7. Seven\n")
	f.file("server/a_test.go", "package server\n\nfunc TestPublish(t *testing.T) {}\n")
	f.ticket("01", "implement", "", "## Test seams\n\n"+
		"`gx tickets nosuch` and `gx tickets add --nosuch`.\n"+
		"`docs/nowhere.md`, `-run 'TestNoSuch'`, `-run 'TestPublish'`.\n"+
		"flow 999, new flow 5, new flow 998, flow 7.\n"+
		"`tickets.Work`, `git rev-parse --short HEAD`, `SKILL.md`.\n"+
		"`server/x.go:42` and `server/a_test.go:TestPublish`.\n")
	out, code := f.lint()
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	wantLines(t, out,
		"miss: 01: gx tickets nosuch", "miss: 01: gx tickets add --nosuch",
		"miss: 01: docs/nowhere.md", "miss: 01: -run 'TestNoSuch'",
		"flow 999", "new flow 5: flow 5 is already",
		"warning: 01: server/x.go:42: name a symbol, not a line")
	wantNoLines(t, out, "TestPublish'`", "998", "tickets.Work", "SKILL.md", "flow 7:", "rev-parse")
	if n := strings.Count(out, "miss: "); n != 6 {
		t.Errorf("%d misses, want 6:\n%s", n, out)
	}
}

func TestLintClaims_WarningAloneExitsZero(t *testing.T) {
	f := newLintFixture(t)
	f.ticket("01", "implement", "", "see `server/x.go:42`\n")
	out, code := f.lint()
	if code != 0 {
		t.Errorf("exit = %d, want 0:\n%s", code, out)
	}
	wantLines(t, out, "name a symbol, not a line")
}

func TestLintClaims_PlannedByBlocker(t *testing.T) {
	f := newLintFixture(t)
	f.ticket("01", "implement", "", "## What to build\n\nAdd `docs/nowhere.md`.\n")
	f.ticket("02", "implement", `["01"]`, "## Test seams\n\n`docs/nowhere.md`\n")
	f.ticket("03", "implement", "", "## Test seams\n\n`docs/nowhere.md`\n")
	out, code := f.lint()
	wantLines(t, out, "planned: 01: ", "miss: 03: ")
	wantNoLines(t, out, "miss: 02", "miss: 01")
	wantLines(t, out, "planned: 02: ")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestLintClaims_VerifyRules(t *testing.T) {
	f := newLintFixture(t)
	f.ticket("01", "implement", "", "## Acceptance criteria\n\n- [ ] `gx tickets publish` works\n")
	f.ticket("02", "verify", "", "## What to verify\n\n`gx tickets publish`, `gx tickets nosuch`, `-run 'TestNoSuch'`\n")
	f.file("a_test.go", "package a\n\nfunc TestOther(t *testing.T) {}\n")
	out, code := f.lint()
	if code != 0 {
		t.Errorf("verify misses exit %d, want 0:\n%s", code, out)
	}
	wantLines(t, out, "warning: 02: gx tickets nosuch", "warning: 02: -run 'TestNoSuch'")
	wantNoLines(t, out, "miss:")

	f.ticket("03", "implement", "", "`gx tickets nosuch`\n")
	if _, code := f.lint(); code != 1 {
		t.Errorf("implement miss exit %d, want 1", code)
	}
}

func TestLintClaims_ManifestUsesEpicTickets(t *testing.T) {
	f := newLintFixture(t)
	f.ticket("01", "implement", "", "## What to build\n\n`docs/nowhere.md` and `gx tickets publish`\n")
	manifest := t.TempDir()
	testutil.WriteFile(t, manifest, "a.md", "## Test seams\n\n`docs/nowhere.md`\n")
	testutil.WriteFile(t, manifest, "v.md", "## What to verify\n\n`gx tickets publish`\n")
	testutil.WriteFile(t, manifest, "m.yaml", "- slug: a\n  type: implement\n  blocked_by: [\"01\"]\n  body: a.md\n- slug: v\n  type: verify\n  body: v.md\n")
	out, code := f.lint("--manifest", filepath.Join(manifest, "m.yaml"))
	if code != 0 {
		t.Errorf("exit = %d:\n%s", code, out)
	}
	wantLines(t, out, "planned: a: ")
}

func TestLintClaims_EpicAndManifestAgree(t *testing.T) {
	body := "## Test seams\n\n`gx tickets nosuch` `docs/nowhere.md` `server/x.go:42`\n"
	f := newLintFixture(t)
	manifest := t.TempDir()
	testutil.WriteFile(t, manifest, "a.md", body)
	testutil.WriteFile(t, manifest, "m.yaml", "- slug: a\n  type: implement\n  body: a.md\n")
	fromManifest, code := f.lint("--manifest", filepath.Join(manifest, "m.yaml"))
	if code != 1 {
		t.Errorf("manifest exit = %d, want 1", code)
	}
	f.ticket("01", "implement", "", body)
	fromEpic, _ := f.lint()
	norm := func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(s, ": 01: ", ": X: "), ": a: ", ": X: ")
	}
	if norm(fromEpic) != norm(fromManifest) {
		t.Errorf("epic output:\n%s\nmanifest output:\n%s", fromEpic, fromManifest)
	}
}

func TestLintClaims_OrderAndSkips(t *testing.T) {
	f := newLintFixture(t)
	f.ticket("01", "implement", "", "## What to build\n\n`docs/planned.md`\n")
	f.ticket("02", "implement", "", "`docs/planned.md` `server/x.go:42` `-run 'TestAny'` new flow 1\n")
	out, _ := f.lint()
	iMiss, iWarn, iPlanned, iSkip := strings.Index(out, "miss:"), strings.Index(out, "warning:"), strings.Index(out, "planned:"), strings.Index(out, "skipped:")
	if iMiss < 0 || !(iMiss < iWarn && iWarn < iPlanned && iPlanned < iSkip) {
		t.Errorf("want misses, warnings, planned, skipped in order:\n%s", out)
	}
	wantLines(t, out, "skipped: -run patterns", "skipped: flow ids")
}

func TestLintClaims_NoGoTestsSkipsAndExitsZero(t *testing.T) {
	f := newLintFixture(t)
	f.ticket("01", "implement", "", "`-run 'TestAny'` new flow 1\n")
	out, code := f.lint()
	if code != 0 {
		t.Errorf("exit = %d, want 0:\n%s", code, out)
	}
	wantNoLines(t, out, "miss:")
}

func TestLintClaims_HeaderHasShortCommit(t *testing.T) {
	f := newLintFixture(t)
	f.ticket("01", "implement", "", "text\n")
	out, _ := f.lint()
	want, err := git.ShortHead(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(out, "\n")
	if !strings.Contains(first, want) || !strings.Contains(first, "working tree") {
		t.Errorf("header %q lacks commit %q or working tree", first, want)
	}
}
