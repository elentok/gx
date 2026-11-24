package skills

import (
	"os"
	"strings"
	"testing"
)

func TestSaveScriptAnchors(t *testing.T) {
	const path = "gx-prototype/gx/save-route-to-html.js"
	checkAnchors(t, path,
		"<style data-mock-inlined>", "data:font/woff2;base64,",
		`querySelectorAll("script").forEach((s) => s.remove())`,
	)
	text := readFile(t, path)
	if !strings.HasPrefix(text, "async page =>") {
		t.Errorf("%s must start with the async page function", path)
	}
	for _, gone := range []string{"<script", "import ", "export ", "require("} {
		if strings.Contains(text, gone) {
			t.Errorf("%s still has %q", path, gone)
		}
	}
}

func TestMockGuideAnchors(t *testing.T) {
	checkAnchors(t, "gx-prototype/MOCK.md",
		"Try the in-app route first (UI.md sub-shape A or B)", "one HTML file",
		"Use a standalone file only when the app cannot run or the mock is an artifact",
		"computed styles with the real page", "line height, disabled opacity and row heights",
		"built from source only",
		"Find the project's own theme file, fonts, components and icons",
		"Never invent colors, use `system-ui` or guess a font size",
		"The project's tokens win over the look the claude.ai artifact design skill pushes",
		"Never redraw an icon", "switcher-snippet.html",
		"<style data-mock-inlined>", "That is the only block the helper exempts",
		"`?variant=` param in every project", "The `#v=` hash is only for a standalone file",
		"same `data-mock-inlined` marker", "## In gx",
		"gx/tokens.css", "gx/base.css", "gx/fonts.css", "gx/save-route-to-html.js", "gx/README.md",
	)
	checkAnchors(t, "gx-prototype/gx/README.md",
		"webui/src/ui/<name>/", "webui/src/gallery/registry.tsx",
		"webui/src/dev/PrototypeSwitcher.tsx", "webui/src/ui/theme.ts",
		"react-icons/lu", "PiNotePencil", "react-icons/pi", "webui/src/design/Composer.tsx",
		"webui/src/page/PageIcon.tsx", "webui/src/shell/SidebarFooter.tsx",
		"Saira and Saira Condensed", "Chakra UI v3", "<style data-mock-inlined>",
	)
}

func TestMockGuideGenericOutsideGx(t *testing.T) {
	head, _, found := strings.Cut(readFile(t, "gx-prototype/MOCK.md"), "## In gx")
	if !found {
		t.Fatal("MOCK.md has no ## In gx section")
	}
	bodies := map[string]string{
		"MOCK.md before ## In gx": head,
		"switcher-snippet.html":   readFile(t, "gx-prototype/switcher-snippet.html"),
	}
	for name, body := range bodies {
		for _, bad := range []string{"gx/", "webui/", "tokens.css", "base.css", "fonts.css", "--chakra", "Chakra", "Saira", "react-icons"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s names gx-specific %q", name, bad)
			}
		}
	}
}

func TestPrototypeStepsWireMockGuide(t *testing.T) {
	checkAnchors(t, "gx-prototype/SKILL.md", "[MOCK.md](MOCK.md)")
	checkAnchors(t, "gx-prototype/UI.md",
		"[MOCK.md](MOCK.md)", "PrototypeSwitcher", "switcher-snippet.html",
		"`?variant=` in every project", "`#v=` is only for a standalone file",
		"[ -d testutil/gxfixture ]", "go run ./testutil/mocklint/cmd <file>...",
		"comparison pairs", "source-only note", "`<style data-mock-inlined>`",
		"the switcher stays and only the variants are dropped",
		"the switcher is dropped too, unless the project keeps a permanent one",
	)
	ui := normalizeSpace(readFile(t, "gx-prototype/UI.md"))
	if strings.Contains(ui, "and the switcher from main") {
		t.Error("gx-prototype/UI.md step 6 still drops the switcher in gx")
	}
	checkAnchors(t, "gx-design/SKILL.md",
		"UI mock follows [MOCK.md](../gx-prototype/MOCK.md)",
		"each stored mock follow `MOCK.md`", "comparison pairs", "source-only note",
		"clean helper result", "not the design session",
	)
}

func TestNoGxMockSkill(t *testing.T) {
	if _, err := os.Stat("gx-mock"); !os.IsNotExist(err) {
		t.Errorf("skills/gx-mock must not exist (stat err: %v)", err)
	}
}

func TestMockGuidanceSwitcherSnippet(t *testing.T) {
	const path = "gx-prototype/switcher-snippet.html"
	checkAnchors(t, path,
		`class="mk-sw"`, `class="prev"`, `class="next"`, `class="mk-label"`,
		`<b>`, `<small>`, `class="mk-tag"`, `>MOCK<`,
		"min-width:48px", "min-height:48px", "calc(100vw - 16px)", "safe-area-inset-bottom",
		"#v=", "data-v", "data-name", "isContentEditable",
	)
	text := readFile(t, path)
	for _, gone := range []string{"var(--", "data-sw", ".seg", "#pick", "/Users/", ".scratch", ".local/share/gx"} {
		if strings.Contains(text, gone) {
			t.Errorf("%s still has %q", path, gone)
		}
	}
}
