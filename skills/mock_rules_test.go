package skills

import (
	"strings"
	"testing"
)

// normalizeSpace collapses every whitespace run (including line wraps) to one space.
func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// checkAnchors fails for each phrase missing from the file, ignoring whitespace differences.
func checkAnchors(t *testing.T, path string, wants ...string) {
	t.Helper()
	text := normalizeSpace(readFile(t, path))
	for _, want := range wants {
		if !strings.Contains(text, normalizeSpace(want)) {
			t.Errorf("%s lacks %q", path, want)
		}
	}
}

func TestMockRulesAnchors(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		wants []string
	}{
		{"mock check", "gx-ui-check/MOCK-CHECK.md", []string{
			"[data-mock-state=<s>] [data-mock-part=<p>]",
			"`[data-mock-state=<s>]`",
			"device scale factor 1",
			"Start the fixture scenario with [gx-ui-check](SKILL.md)",
			"A later decision wins on behaviour",
			"--mock-dir \"$mocks\"",
			"`mock_url`",
			"Never start `python3 -m http.server` yourself",
			"`waitUntil: \"domcontentloaded\"`",
			"Resolve the mock directory first",
			"never set `allowUnrestrictedFileAccess`",
			"from inside the repo worktree",
			"`--filename` an absolute path",
			"`<state>-<part>-built.png`",
			"empty result means the state is unchecked",
			"`pass`, `fail`",
		}},
		{"ui check build", "gx-ui-check/SKILL.md", []string{
			"go run ./testutil/gxfixture [--hold] <scenario>",
			"`-tags gxfixture`",
			"`playwright-cli -s=<browser_session>",
			"`--mock-dir <path>`",
			"`PLAYWRIGHT_BROWSERS_PATH`",
			"Resolve `gx tickets root` before any `cd`",
			"`waitUntil: \"domcontentloaded\"`",
		}},
		{"implement", "gx-implement/SKILL.md", []string{
			"## Mock check",
			"](../gx-ui-check/MOCK-CHECK.md)",
			"$(gx tickets root)/<map>/mocks/",
			"minus `-impl`",
			"blocks `finished`",
			"Skipping the check is never a question to ask a person",
			"## Spec deviations",
			"`gx tickets ask`",
		}},
		{"prototype", "gx-prototype/UI.md", []string{
			"### 5a. Store the approved mock",
			"`<root>/<map>/mocks/<NN>-<slug>.html`",
			"`data-mock-state=\"<name>\"`",
			"`data-mock-width=\"<px>\"`",
			"`data-mock-part=\"<kebab-name>\"`",
		}},
		{"wayfinder", "gx-wayfinder/SKILL.md", []string{
			"\"Store the approved mock\" step in [UI.md](../gx-prototype/UI.md)",
		}},
		{"to-spec", "gx-to-spec/SKILL.md", []string{
			"**Mock states** list",
			"`$(gx tickets root)/<map>/mocks/<NN>-<slug>.html`",
		}},
		{"design", "gx-design/SKILL.md", []string{
			"`<root>/<map>/mocks/`",
			"Mock states line",
		}},
		{"to-tickets", "gx-to-tickets/SKILL.md", []string{
			"## Mock check",
			"- Mock file: `<full path>`",
			"| 8K",
			"add 10000 to `expected_context_window` per mock state",
			"at most 4 mock states per implement ticket",
			"checks the first 4 states",
		}},
		{"verify-epic", "gx-verify-epic/SKILL.md", []string{
			"**Mock states lines.**",
			"[gx-ui-check/MOCK-CHECK.md](../gx-ui-check/MOCK-CHECK.md)",
			"estimate from the gx-to-tickets estimate table",
			"`pass`, `fail`, `unchecked`",
			"A missing mock file named by a Mock states line is an `unchecked` line plus a finding",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkAnchors(t, tc.path, tc.wants...)
		})
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
