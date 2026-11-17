package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// checkRuleHeadings fails when the skill has no `##` rule heading or when a
// rule lacks a Why or Fix line.
func checkRuleHeadings(t *testing.T, path string) {
	t.Helper()
	text := readFile(t, path)
	var headings []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			headings = append(headings, strings.TrimPrefix(line, "## "))
		}
	}
	if len(headings) == 0 {
		t.Fatalf("%s has no rule headings", path)
	}
	for _, h := range headings {
		section := text[strings.Index(text, "## "+h):]
		if i := strings.Index(section[3:], "\n## "); i >= 0 {
			section = section[:i+3]
		}
		for _, part := range []string{"**Why:**", "**Fix:**"} {
			if !strings.Contains(section, part) {
				t.Errorf("%s: rule %q lacks %s", path, h, part)
			}
		}
	}
}

func TestTestGuidelinesHeadings(t *testing.T) {
	checkRuleHeadings(t, "gx-test-guidelines/SKILL.md")
}

func TestGoTestsSkill(t *testing.T) {
	checkRuleHeadings(t, "gx-go-tests/SKILL.md")
	checkAnchors(t, "gx-go-tests/SKILL.md",
		"(../gx-test-guidelines/SKILL.md)",
		"## Readiness signal",
		"## State files",
		"## Process limits",
		"## Cleanups",
		"## Wire fields",
	)
}

func TestPlaywrightTestsSkill(t *testing.T) {
	checkRuleHeadings(t, "gx-playwright-tests/SKILL.md")
	checkAnchors(t, "gx-playwright-tests/SKILL.md",
		"(../gx-test-guidelines/SKILL.md)",
		"`expect.poll`",
		"`error-context.md`",
		"## Read `error-context.md` first",
		"## Assert the destination after you navigate",
		"## Read boxes together in `expect.poll`",
		"## Review renames by diff",
		"## Visible and enabled is not clickable",
		"## A fixture exit and a SecurityError are one failure",
		"## Update the flow when the change was on purpose",
		"## Write an e2e only when a lower test cannot cover it",
		"## A state read follows an `expect` on the same locator",
		"## `-g` patterns match the full title",
		"## Wait for the event, never for time",
		"## A new sibling changes the counts",
		"## Prove a rare flake under load",
		"## Keep each `expect` timeout below its test timeout",
		"## A `check()` that did not change state can be an unmount",
	)
}

func TestVitestSkill(t *testing.T) {
	checkRuleHeadings(t, "gx-vitest-tests/SKILL.md")
	checkAnchors(t, "gx-vitest-tests/SKILL.md",
		"(../gx-test-guidelines/SKILL.md)",
		"## Prefer spyOn over mock",
		"## Blank line between blocks",
		"## Wait on what you read",
		"## Renders share one cache",
		"## Never click a disabled control",
		"## Never fix a timeout with a longer wait",
		"## A product timer needs a disposer",
		"## Prove a race with an injected delay",
		"## Check the whole run",
		"## Use the global async wait",
		"## Name a non-default environment",
		"## Restore real timers",
		"needs a disposer",
	)
}

func TestTestReviewSkill(t *testing.T) {
	checkAnchors(t, "gx-test-review/SKILL.md",
		"## 1. Skip when no test file changed",
		"(../gx-test-guidelines/SKILL.md)",
		".ai/index.md",
		"(../gx-code-review/SKILL.md)",
		"consultant triage",
	)
	text := normalizeSpace(readFile(t, "gx-test-review/SKILL.md"))
	for _, bad := range []string{"go test", "gx check", "gx brief"} {
		if strings.Contains(text, bad) {
			t.Errorf("gx-test-review/SKILL.md names a command to run: %q", bad)
		}
	}
}

const gxOnlySkill = "../.claude/skills/gx-only-e2es/SKILL.md"

func TestGxOnlyE2esSkillIsWired(t *testing.T) {
	dir := "../.claude/skills/gx-only-e2es"
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatalf("skill dir: %v", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("%s must be a real directory", dir)
	}
	text := readFile(t, gxOnlySkill)
	fm, _, _ := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---")
	if !strings.Contains(fm, "name: gx-only-e2es") {
		t.Errorf("frontmatter lacks name: gx-only-e2es")
	}
	for _, want := range []string{"webui/e2e/*.e2e.ts", "playwrightguard", "CATALOG.md"} {
		if !strings.Contains(fm, want) {
			t.Errorf("description lacks %q", want)
		}
	}

	link := "../.agents/skills/gx-only-e2es"
	li, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("agents link: %v", err)
	}
	if li.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s must be a symlink", link)
	}
	got, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("resolve link: %v", err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	if got != want {
		t.Errorf("link resolves to %s, want %s", got, want)
	}

	checkAnchors(t, "../.ai/index.md", "`gx-only-e2es`")
	checkAnchors(t, "../playwrightguard/budget_test.go", "gx-only-e2es")
	checkAnchors(t, "../playwrightguard/playwrightguard_test.go", "gx-only-e2es")
	names, err := BundleSkillNames()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name == "gx-only-e2es" || strings.HasSuffix(name, "playwright-e2es") {
			t.Errorf("bundle lists %s", name)
		}
	}
}

func TestPlaywrightE2esSkillIsWired(t *testing.T) {
	checkAnchors(t, "gx-to-tickets/SKILL.md", "gx-playwright-tests")
	checkAnchors(t, "gx-test-review/SKILL.md", "(../gx-playwright-tests/SKILL.md)")
}

func TestGxUICheckBlocksWorkersByDefault(t *testing.T) {
	checkAnchors(t, "gx-ui-check/SKILL.md", "`serviceWorkers: \"block\"`", "bypass `page.route`")
	checkAnchors(t, "gx-ui-check/SKILL.md", "gxfixture stop <scenario|pid>", "`GXF_PID`", "Never use `pkill -f gxfixture`",
		"until [ -s fx.json ]", "$GXF_WEB_COOKIE", "Resolve it before you source the fixture env")
}

func TestGxImplementTestRules(t *testing.T) {
	checkAnchors(t, "gx-implement/SKILL.md",
		"(../gx-test-guidelines/SKILL.md)",
		"before you write the first one",
		"Before `--iteration-status finished`, check each new or changed test",
		"pause with `gx tickets ask`",
		"There is no exemption list",
		"`gx check --full`",
		"changed-file `gx check`",
		"`gx-playwright-tests`",
	)
	checkAnchors(t, "../.ai/index.md", "`gx check --full`", "skills/gx-test-guidelines/SKILL.md", "GOMAXPROCS", "claudeenv", "envguard")
	checkAnchors(t, "../.ai/index.md", "start it with `run_in_background` from the first call", "TaskStop")
}

func TestUnrelatedFlakyFlowRule(t *testing.T) {
	checkAnchors(t, gxOnlySkill,
		"## An unrelated failing flow: rerun once, file it, stop",
		"`gx-follow-up`",
		"`gx-flake-tickets`",
		"`--repeat-each`",
	)
	checkAnchors(t, "gx-implement/SKILL.md", "gx-playwright-tests", "rerun the flow once")
}

func TestPlaywrightE2esRunsGuardBeforeFirstBrowserRun(t *testing.T) {
	checkAnchors(t, gxOnlySkill,
		"## Before the first browser run",
		"`gx brief go-test ./playwrightguard`",
	)
}

func TestFailingFlowReadsErrorContextFirst(t *testing.T) {
	checkAnchors(t, gxOnlySkill,
		"## A failing flow: read `error-context.md` before anything else",
		"clean worktree at HEAD",
		"`gx tickets ask`",
	)
	checkAnchors(t, "gx-implement/SKILL.md", "read its `error-context.md` first", "`gx-playwright-tests`")
}

func TestTestGuidelinesKeepGxOnlyRulesInIndex(t *testing.T) {
	checkAnchors(t, "../.ai/index.md", "GOMAXPROCS", "claudeenv.In(home)", "envguard/envguard_test.go")
	for _, path := range []string{"gx-test-guidelines/SKILL.md", "gx-go-tests/SKILL.md"} {
		text := normalizeSpace(readFile(t, path))
		for _, bad := range []string{"GOMAXPROCS", "claudeenv", "envguard"} {
			if strings.Contains(text, bad) {
				t.Errorf("%s holds gx-only term %q", path, bad)
			}
		}
	}
}
