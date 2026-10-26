package skills

import (
	"strings"
	"testing"
)

func TestGxImplementMockCheckRules(t *testing.T) {
	impl := readFile(t, "gx-implement/SKILL.md")
	for _, want := range []string{
		"## Mock check",
		"](../gx-ui-check/MOCK-CHECK.md)",
		"$(gx tickets root)/<map>/mocks/",
		"minus `-impl`",
		"No frontmatter link",
		"for each listed state and\n  width",
		"part by part",
		"Check only the parts the ticket built",
		"Fix each look difference",
		"kept on purpose goes under `## Spec deviations`",
		"A named mock file that is missing is a check that cannot run",
		"`gx tickets ask`",
		"Never\n  report a check that did not run",
	} {
		if !strings.Contains(impl, want) {
			t.Errorf("gx-implement/SKILL.md lacks %q", want)
		}
	}

	shared := readFile(t, "gx-ui-check/MOCK-CHECK.md")
	for _, want := range []string{
		"Start the fixture scenario",
		"[data-mock-state=<s>] [data-mock-part=<p>]",
		"You choose how: roles, visible text or a CSS selector",
		"side by side by eye",
		"A later decision wins on behaviour",
		"Compare it with the\nbuilt page at that viewport width",
		"device scale factor 1, cropped to\n   the selector",
	} {
		if !strings.Contains(shared, want) {
			t.Errorf("gx-ui-check/MOCK-CHECK.md lacks %q", want)
		}
	}
}
