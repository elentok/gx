package tickets

import (
	"fmt"
	"image/color"
	"strings"
	"sync"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"

	"github.com/elentok/gx/ui"
)

// ticketGlamourStyle renders a ticket/map body's raw markdown through this
// app's existing Catppuccin Mocha palette rather than one of glamour's
// built-in presets, so it reads as part of the same visual system as the
// rest of the app: no background blocks on headings, bold+color only (see
// ADR 0014).
var ticketGlamourStyle = ansi.StyleConfig{
	Document: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{Color: colorPtr(ui.ColorText)},
	},
	BlockQuote: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{Color: colorPtr(ui.ColorSubtle), Italic: boolPtr(true)},
		Indent:         uintPtr(2),
	},
	List: ansi.StyleList{
		StyleBlock: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: colorPtr(ui.ColorText)}},
	},
	Heading: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{BlockSuffix: "\n", Color: colorPtr(ui.ColorBlue), Bold: boolPtr(true)},
	},
	H1:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
	H2:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
	H3:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
	H4:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
	H5:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
	H6:             ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
	Strikethrough:  ansi.StylePrimitive{CrossedOut: boolPtr(true)},
	Emph:           ansi.StylePrimitive{Color: colorPtr(ui.ColorYellow), Italic: boolPtr(true)},
	Strong:         ansi.StylePrimitive{Color: colorPtr(ui.ColorText), Bold: boolPtr(true)},
	HorizontalRule: ansi.StylePrimitive{Color: colorPtr(ui.ColorSurface1), Format: "\n──────────\n"},
	Item:           ansi.StylePrimitive{BlockPrefix: "• "},
	Enumeration:    ansi.StylePrimitive{BlockPrefix: ". ", Color: colorPtr(ui.ColorSubtle)},
	Task:           ansi.StyleTask{Ticked: "[x] ", Unticked: "[ ] "},
	Link:           ansi.StylePrimitive{Color: colorPtr(ui.ColorBlue), Underline: boolPtr(true)},
	LinkText:       ansi.StylePrimitive{Color: colorPtr(ui.ColorMauve)},
	Image:          ansi.StylePrimitive{Color: colorPtr(ui.ColorBlue), Underline: boolPtr(true)},
	ImageText:      ansi.StylePrimitive{Color: colorPtr(ui.ColorMauve), Format: "Image: {{.text}} →"},
	Code:           ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: colorPtr(ui.ColorTeal)}},
	CodeBlock: ansi.StyleCodeBlock{
		StyleBlock: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: colorPtr(ui.ColorTeal)}},
	},
	DefinitionDescription: ansi.StylePrimitive{BlockPrefix: "\n🠶 "},
}

// colorPtr converts one of ui/styles.go's Catppuccin Mocha color.Color
// constants into the hex string pointer glamour's ansi.StyleConfig expects,
// so this style stays defined in terms of the same palette constants rather
// than a second set of hardcoded hex literals.
func colorPtr(c color.Color) *string {
	nrgba := color.NRGBAModel.Convert(c).(color.NRGBA)
	s := fmt.Sprintf("#%02x%02x%02x", nrgba.R, nrgba.G, nrgba.B)
	return &s
}

func boolPtr(b bool) *bool { return &b }

func uintPtr(u uint) *uint { return &u }

// termRenderers holds one glamour renderer per wrap width: building one is
// costly and the preview re-renders at the same few widths. A TermRenderer is
// not safe for concurrent use, so termRenderersMu also serializes Render.
var (
	termRenderersMu sync.Mutex
	termRenderers   = map[int]*glamour.TermRenderer{}
)

func renderWithCachedRenderer(body string, width int) (string, error) {
	termRenderersMu.Lock()
	defer termRenderersMu.Unlock()
	r, ok := termRenderers[width]
	if !ok {
		var err error
		r, err = glamour.NewTermRenderer(glamour.WithStyles(ticketGlamourStyle), glamour.WithWordWrap(width))
		if err != nil {
			return "", err
		}
		termRenderers[width] = r
	}
	return r.Render(body)
}

// renderTicketMarkdown renders a ticket/map body's raw markdown verbatim
// through glamour, word-wrapped to width. Falls back to the raw body on a
// renderer error (malformed style construction only - never a property of
// the markdown itself) so the preview panel never blanks out.
func renderTicketMarkdown(body string, width int) string {
	if width < 1 {
		width = 1
	}
	out, err := renderWithCachedRenderer(body, width)
	if err != nil {
		return body
	}
	return strings.TrimRight(out, "\n")
}
