package skills

import (
	"strings"
	"testing"
)

var inlinedSummaryFormat = []string{
	"](../gx-local-tracker.md#iteration-summaries)",
	"`### <session id> — <RFC 3339 time",
	"`Did`, `Went wrong`, `Decisions`, `Left`",
	`gx tickets section <addr> "Iteration summaries" -`,
}

var editOnlyRule = []string{"`sed -i`", "Write the file first", "one command per call"}

func TestCheckRulesAnchors(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		wants []string
	}{
		{"failing tests", "gx-failing-tests.md", []string{
			"3 times", "git merge-base", "flake", "pre-existing failure", "branch fault",
			"30 changed lines or fewer in at most 2 files", "](gx-follow-up/SKILL.md)", "--check-failures",
			"More than 10 failing tests", "## Many failures at once", "one grouped follow-up", "--workers=2",
		}},
		{"ui-check build order and repo path", "gx-ui-check/SKILL.md", []string{"make web-build", "GXF_REPO"}},
		{"flake tickets", "gx-flake-tickets.md", []string{"`uptime`", "shared state", "`-run '^TestX$' -count=20`"}},
		{"flake tickets test-health", "gx-flake-tickets.md", []string{"test-health", "seen again"}},
		{"flake tickets in follow-up", "gx-follow-up/SKILL.md", []string{"](../gx-flake-tickets.md)"}},
		{"flake tickets in to-tickets", "gx-to-tickets/SKILL.md", []string{"](../gx-flake-tickets.md)"}},
		{"implement", "gx-implement/SKILL.md", []string{
			"](../gx-failing-tests.md)", "-count=20", "`gx check`",
		}},
		{"bump marks release runs", "gx-bump/SKILL.md", []string{"GX_CHECK_ORIGIN=release"}},
		{"implement guards kind", "gx-implement/SKILL.md", []string{"`gx check --kind guards`"}},
		{"implement known failures", "gx-implement/SKILL.md", []string{"--known-failures", "--check-failures"}},
		{"implement known red", "gx-implement/SKILL.md", []string{"gx tickets follow-up search", "\"known red\" line", "`project:epic/ID`"}},
		{"implement go run", "gx-implement/SKILL.md", []string{"go run ."}},
		{"to-tickets go run", "gx-to-tickets/SKILL.md", []string{"go run ."}},
		{"implement edit-only", "gx-implement/SKILL.md", []string{"`sed -i`", "write the file first"}},
		{"code-review edit-only", "gx-code-review/SKILL.md", []string{"file edit (Edit) or file write (Write) only", "`sed -i`", "`cat >`", "Write the file first"}},
		{"verify edit-only", "gx-verify/SKILL.md", editOnlyRule},
		{"merge edit-only", "gx-merge/SKILL.md", editOnlyRule},
		{"resolving-merge-conflicts edit-only", "gx-resolving-merge-conflicts/SKILL.md", editOnlyRule},
		{"ui-check edit-only", "gx-ui-check/SKILL.md", editOnlyRule},
		{"mock-check edit-only", "gx-ui-check/MOCK-CHECK.md", editOnlyRule},
		{"mock-check bounded search", "gx-ui-check/MOCK-CHECK.md", []string{"`rg -o '.{0,80}PATTERN.{0,80}' <mock>`"}},
		{"follow-up edit-only", "gx-follow-up/SKILL.md", editOnlyRule},
		{"flake tickets edit-only", "gx-flake-tickets.md", editOnlyRule},
		{"verify-epic edit-only", "gx-verify-epic/SKILL.md", editOnlyRule},
		{"to-tickets edit-only", "gx-to-tickets/SKILL.md", editOnlyRule},
		{"code-review shared diff", "gx-code-review/SKILL.md", []string{
			"### Shared review file", "scratchpad", "verify a suspicion", "### Size gate", "50 changed lines",
			"gx tickets review-bundle <project:epic>", "`deviations.md`",
		}},
		{"code-review per-area files, spec path and coverage", "gx-code-review/SKILL.md", []string{
			"one diff file per", "100 KB", "minus a trailing -impl", "`Covered:", "Spec reviewed N of M code-changing tickets", "Covered: <addresses reviewed> of <M> code-changing tickets", "`implement` tickets plus their fix children", "`gx tickets root`", "**deferred**, never **approved**",
		}},
		{"code-review test gaps and verified facts", "gx-code-review/SKILL.md", []string{
			"Open verify tickets own test gaps", "no open verify ticket covers that story", "consultant's verified facts",
		}},
		{"code-review follow-up target", "gx-code-review/SKILL.md", []string{
			"real project name", "[routing test](../gx-local-tracker.md)", "Never write the word `project`", "category unchanged", "Never replace them",
		}},
		{"tdd", "gx-tdd/SKILL.md", []string{"](../gx-failing-tests.md)", "-count=20"}},
		{"verify", "gx-verify/SKILL.md", []string{
			"gx check --staged", "`0`: pass", "`1`: fail", "`2`: could not run", "`3`: no applicable checks",
		}},
		{"merge", "gx-merge/SKILL.md", []string{"gx check --since <target>", "](../gx-failing-tests.md)"}},
		{"merge small drift", "gx-merge/SKILL.md", []string{"### Small test drift", "10 lines or fewer", "## Spec deviations"}},
		{"verify-epic", "gx-verify-epic/SKILL.md", []string{
			"gx check --full", "](../gx-failing-tests.md)", "git merge-base main HEAD", "pre-existing, <follow-up addr>",
		}},
		{"verify-epic no other runs during background check", "gx-verify-epic/SKILL.md", []string{
			"start no other test, build or e2e run", "Do not `sleep` and poll",
		}},
		{"verify points to background rule", "gx-verify/SKILL.md", []string{
			"start no other test, build or e2e run", "](../gx-verify-epic/SKILL.md)",
		}},
		{"verify-epic look fix mock check", "gx-verify-epic/SKILL.md", []string{
			"look or layout", "`## Mock check` section", "mock path plus the width",
		}},
		{"verify-epic verified", "gx-verify-epic/SKILL.md", []string{
			"### Verified section", "`## Verified`", "`- <story> — <verdict> @ <commit>`", "equals the current HEAD",
		}},
		{"iteration summaries", "gx-local-tracker.md", []string{
			"## Iteration summaries", "`Did`, `Went wrong`, `Decisions`, `Left`", "labelled `gx`", "never blocks finishing",
		}},
		{"implement iteration summaries", "gx-implement/SKILL.md", inlinedSummaryFormat},
		{"verify-epic iteration summaries", "gx-verify-epic/SKILL.md", inlinedSummaryFormat},
		{"merge iteration summaries", "gx-merge/SKILL.md", inlinedSummaryFormat},
		{"code-review iteration summaries", "gx-code-review/SKILL.md", inlinedSummaryFormat},
		{"research", "gx-research/SKILL.md", []string{"`gx check`"}},
		{"investigate", "gx-investigate/SKILL.md", []string{
			"`gx check`", "**Write files with Write, in their own call.**", "never end on an \"Open question\"",
		}},
		{"to-tickets", "gx-to-tickets/SKILL.md", []string{"iteration hint", "`gx brief`", "`go_symbol_references`", "verified at {{commit}}", "not verified", "Generated files", "`make web-types`", "Test environment", "@vitest-environment", "Playwright filters", "Mock path", "must fail to compile or fail", "Examples against the helper", "`gx brief -C webui vitest <paths>`", "`gx brief -C webui tsc`"}},
		{"to-tickets changed label names e2e flows", "gx-to-tickets/SKILL.md", []string{"name the e2e flows to run"}},
		{"to-tickets bundled and implement-only seams", "gx-to-tickets/SKILL.md", []string{"Every `implement` ticket", "5 or more"}},
		{"to-tickets lint", "gx-to-tickets/SKILL.md", []string{"lint-claims", "--allow-claim-misses", "name a symbol"}},
		{"to-tickets premises", "gx-to-tickets/SKILL.md", []string{"](premises.md)", "name the flow number or test name"}},
		{"to-tickets premises rules", "gx-to-tickets/premises.md", []string{"# Check the premises", "Run it on the base", "`unverified`", "Search strings, not line numbers", "Shared file ordering"}},
		{"to-tickets estimate", "gx-to-tickets/SKILL.md", []string{"1.12", "1.28", "0.83", "60K"}},
		{"to-tickets calibration", "gx-to-tickets/SKILL.md", []string{"gx tickets calibration", "0.83"}},
		{"to-tickets publish", "gx-to-tickets/SKILL.md", []string{"gx tickets publish", "--manifest", "--dry-run", "{{slug}}"}},
		{"to-tickets 4c literal commands and helper fit", "gx-to-tickets/SKILL.md", []string{"**Literal commands**", "**Seam commands are dry-run**", "**Reused helpers fit**", "**Criteria vs must-keep-passing tests**"}},
		{"to-tickets 4c helper rule and slug ID", "gx-to-tickets/SKILL.md", []string{"**Names a seam or hint relies on**", "seam that finds nothing is not a seam", "that ticket's ID."}},
		{"implement edit rules", "gx-implement/SKILL.md", []string{
			"**Make one Edit per contiguous region, not one per line.**",
			"**Send file edits to one file one after another, never in parallel.**",
		}},
		{"implement wip, stdin and retry", "gx-implement/SKILL.md", []string{
			"**A WIP commit before a long background run is allowed.**",
			"`</dev/null`",
			"The finishing commit still needs `gx check` first.",
			"retry the same command once",
		}},
		{"implement spec deviation triggers", "gx-implement/SKILL.md", []string{
			"weaken or loosen an assertion", "unresolved product bug", "say so before finishing", "never only in the final message",
		}},
		{"follow-up", "gx-follow-up/SKILL.md", []string{"search the candidates for that test name first"}},
		{"follow-up seen again", "gx-follow-up/SKILL.md", []string{"gx tickets section --append <addr> Evidence -"}},
		{"follow-up test-health", "gx-follow-up/SKILL.md", []string{"test-health", "gx tickets show <project>:test-health", "gx tickets root"}},
		{"evolve", "gx-evolve/SKILL.md", []string{
			"### Check rows", "### Launch rows", "`slower:`", "`flake:`", "](../gx-follow-up/SKILL.md)", "Category `bug`", "Category `idea`",
		}},
		{"index", "../.ai/index.md", []string{"Verify with `gx check` in the foreground, unpiped", "## Shell is zsh", "${=var}"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { checkAnchors(t, c.path, c.wants...) })
	}
}

func TestSectionReadUsesTicketsFlag(t *testing.T) {
	checkAnchors(t, "gx-verify-epic/SKILL.md",
		"gx tickets show <epic> --section Results --tickets 31-46", "A hand-built loop over")
	checkAnchors(t, "gx-code-review/SKILL.md",
		`gx tickets show <epic> --section "Spec deviations"`, "--tickets", "A hand-built loop over")
}

func TestCheckRulesAbsences(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		unwant []string
	}{
		{"implement", "gx-implement/SKILL.md", []string{"whole package once at the end", "gofmt -l"}},
		{"implement edit rules", "gx-implement/SKILL.md", []string{"each call is a turn", "one failed edit makes the rest of the batch fail"}},
		{"to-tickets stamp and seams", "gx-to-tickets/SKILL.md", []string{"verified at <commit>", "git rev-parse --short HEAD", "Every ticket must declare"}},
		{"to-tickets lint", "gx-to-tickets/SKILL.md", []string{"CLI commands the ticket quotes", "go test -list"}},
		{"to-tickets estimate", "gx-to-tickets/SKILL.md", []string{"for the whole session", "median of 2 times", "check their actual cost"}},
		{"to-tickets publish", "gx-to-tickets/SKILL.md", []string{
			"gx tickets epic add <project:epic> --kind loop --status draft",
			"gx tickets add <epic-slug> --slug <slug> --type implement",
		}},
		{"verify", "gx-verify/SKILL.md", []string{"## Check Table"}},
		{"merge", "gx-merge/SKILL.md", []string{"touches neither the test file"}},
		{"verify-epic", "gx-verify-epic/SKILL.md", []string{"CI does that"}},
		{"index", "../.ai/index.md", []string{"make test-server", "-timeout 60s"}},
	}
	for _, c := range cases {
		text := normalizeSpace(readFile(t, c.path))
		for _, bad := range c.unwant {
			if strings.Contains(text, bad) {
				t.Errorf("%s still holds %q", c.path, bad)
			}
		}
	}
}
