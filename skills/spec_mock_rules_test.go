package skills

import (
	"strings"
	"testing"
)

func TestGxToSpecMockStates(t *testing.T) {
	text := readFile(t, "gx-to-spec/SKILL.md")
	for _, want := range []string{
		"end `## Verification` with a\n**Mock states** list",
		"one line per approved state in each stored mock",
		"`$(gx tickets root)/<map>/mocks/<NN>-<slug>.html`",
		"- the state name",
		"- the width",
		"- the fixture scenario",
		"- the part names in that state",
		"Story entries refer to a Mock states line by its state name",
		"They do not repeat the look check",
		"fixture scenario does not exist yet, also list it under the new flows or scenarios to write",
		"With no stored mock, write no Mock states list",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-to-spec/SKILL.md lacks %q", want)
		}
	}
}

func TestGxDesignMockStates(t *testing.T) {
	text := readFile(t, "gx-design/SKILL.md")
	for _, want := range []string{
		"seed it with the list of stored mocks under `<root>/<map>/mocks/`",
		"Does each state of each stored mock have a Mock states line",
		"A missing line\n  is a finding",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-design/SKILL.md lacks %q", want)
		}
	}
}
