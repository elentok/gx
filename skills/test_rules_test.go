package skills

import (
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

func TestPlaywrightE2esSkillIsWired(t *testing.T) {
	checkAnchors(t, "gx-playwright-e2es/SKILL.md", "(../gx-test-guidelines/SKILL.md)", "(../gx-ui-check/SKILL.md)")
	checkAnchors(t, "gx-to-tickets/SKILL.md", "gx-playwright-e2es")
	checkAnchors(t, "gx-test-review/SKILL.md", "(../gx-playwright-e2es/SKILL.md)")
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
		"`gx-playwright-e2es`",
	)
	checkAnchors(t, "../.ai/index.md", "`gx check --full`", "skills/gx-test-guidelines/SKILL.md", "GOMAXPROCS", "claudeenv", "envguard")
	checkAnchors(t, "../.ai/index.md", "start it with `run_in_background` from the first call", "TaskStop")
}

func TestUnrelatedFlakyFlowRule(t *testing.T) {
	checkAnchors(t, "gx-playwright-e2es/SKILL.md",
		"## An unrelated failing flow: rerun once, file it, stop",
		"[gx-follow-up](../gx-follow-up/SKILL.md)",
		"[gx-flake-tickets](../gx-flake-tickets.md)",
		"`--repeat-each`",
	)
	checkAnchors(t, "gx-implement/SKILL.md", "gx-playwright-e2es", "rerun the flow once")
}

func TestPlaywrightE2esRunsGuardBeforeFirstBrowserRun(t *testing.T) {
	checkAnchors(t, "gx-playwright-e2es/SKILL.md",
		"## Before the first browser run",
		"`gx brief go-test ./playwrightguard`",
	)
}

func TestFailingFlowReadsErrorContextFirst(t *testing.T) {
	checkAnchors(t, "gx-playwright-e2es/SKILL.md",
		"## A failing flow: read `error-context.md` before anything else",
		"clean worktree at HEAD",
		"`gx tickets ask`",
	)
	checkAnchors(t, "gx-implement/SKILL.md", "read its `error-context.md` first", "`gx-playwright-e2es`")
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
