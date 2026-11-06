package skills

import (
	"strings"
	"testing"
)

func TestCheckRulesAnchors(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		wants []string
	}{
		{"failing tests", "gx-failing-tests.md", []string{
			"3 times", "git merge-base", "flake", "pre-existing failure", "branch fault",
			"30 changed lines or fewer in at most 2 files", "](gx-follow-up/SKILL.md)", "--check-failures",
		}},
		{"flake tickets", "gx-flake-tickets.md", []string{"`uptime`", "shared state", "`-run '^TestX$' -count=20`"}},
		{"flake tickets in follow-up", "gx-follow-up/SKILL.md", []string{"](../gx-flake-tickets.md)"}},
		{"flake tickets in to-tickets", "gx-to-tickets/SKILL.md", []string{"](../gx-flake-tickets.md)"}},
		{"implement", "gx-implement/SKILL.md", []string{
			"](../gx-failing-tests.md)", "-count=20", "`gx check`",
		}},
		{"implement edit-only", "gx-implement/SKILL.md", []string{"`sed -i`", "write the file first"}},
		{"code-review edit-only", "gx-code-review/SKILL.md", []string{"file edit (Edit) or file write (Write) only", "`sed -i`", "`cat >`", "Write the file first"}},
		{"code-review shared diff", "gx-code-review/SKILL.md", []string{
			"### Shared review file", "scratchpad", "verify a suspicion", "### Size gate", "50 changed lines",
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
		{"research", "gx-research/SKILL.md", []string{"`gx check`"}},
		{"investigate", "gx-investigate/SKILL.md", []string{"`gx check`"}},
		{"to-tickets", "gx-to-tickets/SKILL.md", []string{"iteration hint", "`gx brief`", "`go_symbol_references`", "verified at {{commit}}", "not verified", "Generated files", "`make web-types`", "Test environment", "@vitest-environment", "Playwright filters", "Mock path", "must fail to compile or fail", "Examples against the helper"}},
		{"to-tickets bundled and implement-only seams", "gx-to-tickets/SKILL.md", []string{"Every `implement` ticket", "5 or more"}},
		{"to-tickets lint", "gx-to-tickets/SKILL.md", []string{"lint-claims", "--allow-claim-misses", "name a symbol"}},
		{"to-tickets estimate", "gx-to-tickets/SKILL.md", []string{"1.12", "1.28", "0.83", "60K"}},
		{"to-tickets calibration", "gx-to-tickets/SKILL.md", []string{"gx tickets calibration", "0.83"}},
		{"to-tickets publish", "gx-to-tickets/SKILL.md", []string{"gx tickets publish", "--manifest", "--dry-run", "{{slug}}"}},
		{"to-tickets 4c helper rule and slug ID", "gx-to-tickets/SKILL.md", []string{"**Names a seam or hint relies on**", "seam that finds nothing is not a seam", "that ticket's ID."}},
		{"implement edit rules", "gx-implement/SKILL.md", []string{
			"**Make one Edit per contiguous region, not one per line.**",
			"**Send file edits to one file one after another, never in parallel.**",
		}},
		{"follow-up", "gx-follow-up/SKILL.md", []string{"search the candidates for that test name first"}},
		{"evolve", "gx-evolve/SKILL.md", []string{
			"### Check rows", "### Launch rows", "`slower:`", "`flake:`", "](../gx-follow-up/SKILL.md)", "Category `bug`", "Category `idea`",
		}},
		{"index", "../.ai/index.md", []string{"Verify with `gx check` in the foreground, unpiped"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { checkAnchors(t, c.path, c.wants...) })
	}
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
