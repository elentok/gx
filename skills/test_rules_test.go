package skills

import (
	"strings"
	"testing"
)

func TestTestGuidelinesHeadings(t *testing.T) {
	text := readFile(t, "gx-test-guidelines.md")
	var headings []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			headings = append(headings, strings.TrimPrefix(line, "## "))
		}
	}
	if len(headings) != 11 {
		t.Fatalf("gx-test-guidelines.md has %d rule headings, want 11: %q", len(headings), headings)
	}
	for _, h := range headings {
		section := text[strings.Index(text, "## "+h):]
		if i := strings.Index(section[3:], "\n## "); i >= 0 {
			section = section[:i+3]
		}
		for _, part := range []string{"**Why:**", "**Fix:**"} {
			if !strings.Contains(section, part) {
				t.Errorf("rule %q lacks %s", h, part)
			}
		}
	}
}

func TestTestReviewSkill(t *testing.T) {
	checkAnchors(t, "gx-test-review/SKILL.md",
		"## 1. Skip when no test file changed",
		"(../gx-test-guidelines.md)",
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
	checkAnchors(t, "gx-playwright-e2es/SKILL.md", "(../gx-test-guidelines.md)", "(../gx-ui-check/SKILL.md)")
	checkAnchors(t, "gx-to-tickets/SKILL.md", "gx-playwright-e2es")
	checkAnchors(t, "gx-test-review/SKILL.md", "(../gx-playwright-e2es/SKILL.md)")
}

func TestGxUICheckBlocksWorkersByDefault(t *testing.T) {
	checkAnchors(t, "gx-ui-check/SKILL.md", "`serviceWorkers: \"block\"`", "bypass `page.route`")
}

func TestGxImplementTestRules(t *testing.T) {
	checkAnchors(t, "gx-implement/SKILL.md",
		"(../gx-test-guidelines.md)",
		"before you write the first one",
		"Before `--iteration-status finished`, check each new or changed test",
		"pause with `gx tickets ask`",
		"There is no exemption list",
		"`gx check --full`",
		"changed-file `gx check`",
		"`gx-playwright-e2es`",
	)
	checkAnchors(t, "../.ai/index.md", "`gx check --full`", "skills/gx-test-guidelines.md", "GOMAXPROCS", "claudeenv", "envguard")
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
	guidelines := normalizeSpace(readFile(t, "gx-test-guidelines.md"))
	for _, bad := range []string{"GOMAXPROCS", "claudeenv", "envguard"} {
		if strings.Contains(guidelines, bad) {
			t.Errorf("gx-test-guidelines.md holds gx-only term %q", bad)
		}
	}
}
