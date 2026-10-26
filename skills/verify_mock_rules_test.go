package skills

import (
	"strings"
	"testing"
)

func TestGxVerifyEpicMockStatesRules(t *testing.T) {
	text := readFile(t, "gx-verify-epic/SKILL.md")
	for _, want := range []string{
		"the line word for word (mock path, state,\n  width, scenario, parts), and its 8K estimate",
		"**Mock states lines.**",
		"apply only when the part (or lone ticket) has Mock states lines",
		"the epic verifies as before",
		"Start the line's fixture scenario",
		"screenshot the whole state section\n  of the mock and the built page at the line's width",
		"compare them by eye",
		"[gx-ui-check/MOCK-CHECK.md](../gx-ui-check/MOCK-CHECK.md)",
		"do not copy them here",
		"The\n  one exception is a Mock states line",
		"device scale factor 1, cropped to the state selector",
		"one result line per Mock states line",
		"(`pass`, `fail`,\n  `unchecked`)",
		"A difference listed in the epic's `## Spec deviations` is not a finding",
		"Any other look difference is a finding",
		"it gets a fix ticket",
		"The Mock states list is part of the Verification section",
		"Each Mock states line gets a row too",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-verify-epic/SKILL.md lacks %q", want)
		}
	}
}
