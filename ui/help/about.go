package help

import (
	"strings"

	"github.com/elentok/gx/ui"
)

// renderAbout is the About section followed by a blank line, or "" when there
// is no provider.
func renderAbout(about func() []ui.AboutLine) string {
	if about == nil {
		return ""
	}
	lines := about()
	labelW := 0
	for _, l := range lines {
		labelW = max(labelW, len(l.Label))
	}
	var b strings.Builder
	b.WriteString(ui.StyleHelpHeading.Render("About") + "\n")
	for _, l := range lines {
		label := l.Label + strings.Repeat(" ", labelW-len(l.Label))
		b.WriteString("  " + ui.StyleTitle.Render(label) + "  " + ui.StyleHint.Render(l.Value) + "\n")
	}
	return b.String() + "\n"
}
