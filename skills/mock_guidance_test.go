package skills

import (
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
