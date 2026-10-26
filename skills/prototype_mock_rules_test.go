package skills

import (
	"strings"
	"testing"
)

func TestPrototypeUIStoresApprovedMock(t *testing.T) {
	text := readFile(t, "gx-prototype/UI.md")
	for _, want := range []string{
		"### 5a. Store the approved mock",
		"just before it records the Resolution",
		"`<root>/<map>/mocks/<NN>-<slug>.html`",
		"`gx tickets root`",
		"Never store it in the repo or the `-impl` branch",
		"Keep only the approved rows and states. No rejected variants.",
		"copy the approved rows and states from its HTML into a new standalone file, unchanged in look",
		"The artifact stays as it is",
		"open the approved variant in playwright-cli at each approved state and width",
		"inlines every CSS rule from `document.styleSheets` and drops all scripts",
		"`data-mock-state=\"<name>\"`",
		"`data-mock-width=\"<px>\"`",
		"drawn inside a box of that width",
		"one name at one width",
		"State names are unique within the file",
		"`data-mock-part=\"<kebab-name>\"`",
		"unique within its state section",
		"The same part keeps the same name in every state",
		"The Resolution links both the stored file and the artifact",
		"is the approved mock even if someone edits the artifact later, until a map ticket approves the change",
		"says so in its Resolution and deletes the old file",
		"a variant route made for a design map never folds into main",
		"the stored mock and that branch are the outputs",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-prototype/UI.md lacks %q", want)
		}
	}
}

func TestPrototypeLogicHasNoStoreStep(t *testing.T) {
	text := readFile(t, "gx-prototype/LOGIC.md")
	for _, banned := range []string{"Store the approved mock", "data-mock-", "/mocks/"} {
		if strings.Contains(text, banned) {
			t.Errorf("gx-prototype/LOGIC.md has %q", banned)
		}
	}
}

func TestWayfinderPrototypePointsAtStoreStep(t *testing.T) {
	text := readFile(t, "gx-wayfinder/SKILL.md")
	for _, want := range []string{
		`"Store the approved mock" step in [UI.md](../gx-prototype/UI.md)`,
		"before recording the Resolution",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("gx-wayfinder/SKILL.md lacks %q", want)
		}
	}
}
