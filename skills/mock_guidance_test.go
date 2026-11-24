package skills

import (
	"strings"
	"testing"
)

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
