package skills

import (
	"strings"
	"testing"
)

func TestMockCheckRules(t *testing.T) {
	text := readFile(t, "gx-ui-check/MOCK-CHECK.md")
	for _, want := range []string{
		"Start the fixture scenario with [gx-ui-check](SKILL.md)",
		"`file://` URL",
		"Do not start a local\n   server",
		"`[data-mock-state=<s>] [data-mock-part=<p>]`",
		"`[data-mock-state=<s>]`",
		"at the state's width, at device scale factor 1, cropped to\n   the selector",
		"Never take a full page at retina scale",
		"You choose how",
		"side by side by eye",
		"List each difference",
		"The mock wins on layout, spacing, sizes, colours and type",
		"A later decision wins on behaviour",
		"Pixel-perfect is not a goal",
		"fixed-width box in the mock, where media queries do not fire",
		"at that viewport width",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-ui-check/MOCK-CHECK.md lacks %q", want)
		}
	}
}
