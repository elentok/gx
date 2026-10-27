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
		{"implement", "gx-implement/SKILL.md", []string{
			"](../gx-failing-tests.md)", "-count=20", "`gx check`",
		}},
		{"tdd", "gx-tdd/SKILL.md", []string{"](../gx-failing-tests.md)", "-count=20"}},
		{"verify", "gx-verify/SKILL.md", []string{
			"gx check --staged", "`0`: pass", "`1`: fail", "`2`: could not run", "`3`: no applicable checks",
		}},
		{"merge", "gx-merge/SKILL.md", []string{"gx check --since <target>", "](../gx-failing-tests.md)"}},
		{"verify-epic", "gx-verify-epic/SKILL.md", []string{
			"gx check --full", "](../gx-failing-tests.md)", "git merge-base main HEAD", "pre-existing, <follow-up addr>",
		}},
		{"research", "gx-research/SKILL.md", []string{"`gx check`"}},
		{"investigate", "gx-investigate/SKILL.md", []string{"`gx check`"}},
		{"to-tickets", "gx-to-tickets/SKILL.md", []string{"iteration hint", "`gx brief`"}},
		{"follow-up", "gx-follow-up/SKILL.md", []string{"search the candidates for that test name first"}},
		{"evolve", "gx-evolve/SKILL.md", []string{
			"### Check rows", "`slower:`", "`flake:`", "](../gx-follow-up/SKILL.md)", "Category `bug`", "Category `idea`",
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
