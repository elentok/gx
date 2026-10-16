package help

import (
	"strings"
	"sync/atomic"

	"github.com/elentok/gx/ui"
)

// AboutLine is one label/value row of the help dialog's About section.
type AboutLine struct {
	Label, Value string
}

// The app owns the state About shows (server, split app), while every page
// builds its own help model, so a package-level hook lets the app register it
// once instead of wiring it through each page. Atomic because parallel tests
// each start an app that registers its own.
var aboutProvider atomic.Pointer[func() []AboutLine]

// SetAboutProvider registers the function that returns the About section's
// current lines; nil removes it.
func SetAboutProvider(f func() []AboutLine) {
	if f == nil {
		aboutProvider.Store(nil)
		return
	}
	aboutProvider.Store(&f)
}

// renderAbout is the About section followed by a blank line, or "" when no
// provider is registered.
func renderAbout() string {
	f := aboutProvider.Load()
	if f == nil {
		return ""
	}
	lines := (*f)()
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
