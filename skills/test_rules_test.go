package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

func TestVerifyEpicRunsFullCheckRequireQuiet(t *testing.T) {
	checkAnchors(t, "gx-verify-epic/SKILL.md", "gx check --full --require-quiet")
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
		"(../gx-go-tests/SKILL.md)",
		"(../gx-vitest-tests/SKILL.md)",
		"(../gx-playwright-tests/SKILL.md)",
		"**Meaningful coverage:**",
		"**Assertions that pass early:**",
		"**Timing assumptions:**",
		"**Isolation:**",
		"**Cleanup:**",
		"**Diagnostics:**",
		"needs a disposer",
	)
	text := normalizeSpace(readFile(t, "gx-test-review/SKILL.md"))
	for _, bad := range []string{"go test", "gx check", "gx brief", "gx-playwright-e2es"} {
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
}

func TestGxUICheckBlocksWorkersByDefault(t *testing.T) {
	checkAnchors(t, "gx-ui-check/SKILL.md", "`serviceWorkers: \"block\"`", "bypass `page.route`")
	checkAnchors(t, "gx-ui-check/SKILL.md", "gxfixture stop <scenario|pid>", "`GXF_PID`", "Never use `pkill -f gxfixture`",
		"until [ -s fx.json ]", "$GXF_WEB_COOKIE", "Resolve it before you source the fixture env")
	checkAnchors(t, "gx-ui-check/SKILL.md", "`pages`", "`GXF_PAGE_<NAME>`", "cookie-set", "`$GXF_PAGE_DRAFT_1`",
		"waitFor()", "`--raw`")
}

func TestSingleFlowRunRoute(t *testing.T) {
	for _, path := range []string{"gx-ui-check/SKILL.md", "../webui/e2e/CATALOG.md"} {
		checkAnchors(t, path, "gx brief -C webui playwright <file> <flow numbers>", "pnpm run e2e -- <filter>", "unreliable")
		if strings.Contains(normalizeSpace(readFile(t, path)), "Playwright's `-g <flow>`") {
			t.Errorf("%s still tells people to pass -g to make web-e2e", path)
		}
	}
}

func TestGxImplementTestRules(t *testing.T) {
	checkAnchors(t, "gx-implement/SKILL.md",
		"(../gx-test-guidelines/SKILL.md)",
		"(../gx-go-tests/SKILL.md)",
		"(../gx-vitest-tests/SKILL.md)",
		"(../gx-playwright-tests/SKILL.md)",
		".ai/index.md",
		"before you write the first one",
		"Before `--iteration-status finished`, check each new or changed test",
		"pause with `gx tickets ask`",
		"There is no exemption list",
		"`gx check --full`",
		"changed-file `gx check`",
		"`rg` the e2e files",
		"the `gx-only-e2es` skill",
		"`gx brief -C webui playwright`",
		"Every new test needs a red run",
		"packages that import the changed package",
		"Do not extend a shared test fixture",
		"`gx tickets set --status done` is refused",
		"use `gx tickets close`",
		"`gx brief tsc`",
	)
	checkAnchors(t, "gx-verify-epic/SKILL.md",
		"`gx tickets set --status done` is refused",
		"use `gx tickets close`",
	)
	for _, skill := range []string{"gx-implement/SKILL.md", "gx-tdd/SKILL.md"} {
		if strings.Contains(readFile(t, skill), "gx-playwright-e2es") {
			t.Errorf("%s still names gx-playwright-e2es", skill)
		}
	}
	checkAnchors(t, "gx-tdd/SKILL.md",
		"(../gx-test-guidelines/SKILL.md)",
		"(../gx-go-tests/SKILL.md)",
		"(../gx-vitest-tests/SKILL.md)",
		"(../gx-playwright-tests/SKILL.md)",
		".ai/index.md",
	)
	for _, skill := range []string{"gx-tdd/SKILL.md", "gx-to-tickets/SKILL.md"} {
		checkAnchors(t, skill, "Spec deviations", "cannot be shown")
	}
	if strings.Contains(readFile(t, "gx-implement/SKILL.md"), "If you cannot make it fail, pause with `gx tickets ask`") {
		t.Error("gx-implement still tells the agent to ask when a red cannot be shown")
	}
	checkAnchors(t, "gx-implement/SKILL.md", "cannot be shown")
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
	checkAnchors(t, "gx-implement/SKILL.md", "(../gx-playwright-tests/SKILL.md)", ".ai/index.md", "rerun the flow once")
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
	checkAnchors(t, "gx-implement/SKILL.md", "read its `error-context.md` first", "(../gx-playwright-tests/SKILL.md)", ".ai/index.md")
}

var portableTestSkills = []string{"gx-test-guidelines", "gx-go-tests", "gx-vitest-tests", "gx-playwright-tests"}

var gxOnlyTerms = []string{"GOMAXPROCS", "claudeenv", "envguard", "gx-only-e2es", ".ai/index.md", "scripts/"}

// A "gx" followed by a space and a lowercase word is a command. "gx-name"
// (hyphen) is a skill name and "gx." or "gx Foo" is prose.
var gxCommandRe = regexp.MustCompile(`\bgx [a-z]`)

// gxOnlyTextHits returns "file:line: what" for each gx-only text found in the
// given files (name -> text).
func gxOnlyTextHits(files map[string]string) []string {
	var hits []string
	for name, text := range files {
		for i, line := range strings.Split(text, "\n") {
			if m := gxCommandRe.FindString(line); m != "" {
				hits = append(hits, fmt.Sprintf("%s:%d: gx command %q", name, i+1, m))
			}
			for _, term := range gxOnlyTerms {
				if strings.Contains(line, term) {
					hits = append(hits, fmt.Sprintf("%s:%d: gx-only text %q", name, i+1, term))
				}
			}
		}
	}
	sort.Strings(hits)
	return hits
}

func TestTestGuidelinesKeepGxOnlyRulesInIndex(t *testing.T) {
	checkAnchors(t, "../.ai/index.md", "GOMAXPROCS", "claudeenv.In(home)", "envguard/envguard_test.go")
}

// Each portable Go test rule has one home, the Go tests skill. The index keeps the gx helpers.
func TestPortableGoRulesLiveInGoTestsSkill(t *testing.T) {
	checkAnchors(t, "../.ai/index.md", "skills/gx-go-tests/SKILL.md", "skills/gx-test-guidelines/SKILL.md",
		"shortTempDir", "servertest", "scripts/proc-cap", "GX_PROC_HEADROOM", "800")
	index := normalizeSpace(readFile(t, "../.ai/index.md"))
	for _, gone := range []string{"t.TempDir", `-exec "$PWD/scripts/proc-cap"`} {
		if strings.Contains(index, gone) {
			t.Errorf("../.ai/index.md still holds %q", gone)
		}
	}
	checkAnchors(t, "gx-go-tests/SKILL.md", "## Test state", "## Socket paths", "## Clocks",
		"## Real machine state", "## Fakes on idle paths")
}

func TestPortableTestSkillsHoldNoGxOnlyText(t *testing.T) {
	files := map[string]string{}
	for _, dir := range portableTestSkills {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			files[path] = readFile(t, path)
		}
	}
	for _, hit := range gxOnlyTextHits(files) {
		t.Error(hit)
	}
}

func TestPortableSkillGuardCatchesBadInput(t *testing.T) {
	bad := "run `gx brief go-test ./x`\nthen gx check\nsee scripts/proc-cap\nsee .ai/index.md\nuse gx-only-e2es\nset GOMAXPROCS"
	got := gxOnlyTextHits(map[string]string{"bad.md": bad})
	for _, want := range []string{"bad.md:1: gx command", "bad.md:2: gx command", "bad.md:3:", "bad.md:4:", "bad.md:5:", "bad.md:6:"} {
		found := false
		for _, h := range got {
			found = found || strings.HasPrefix(h, want)
		}
		if !found {
			t.Errorf("no hit starting %q in %v", want, got)
		}
	}
	good := "load gx-playwright-tests and gx-test-review.\nThe gx server is not a command line.\nGX Check and gx."
	// "gx server" matches by design: a lowercase word after "gx" reads as a command.
	if got := gxOnlyTextHits(map[string]string{"good.md": good}); len(got) != 1 || !strings.HasPrefix(got[0], "good.md:2:") {
		t.Errorf("good text hits = %v, want only line 2", got)
	}
}
