package diffview

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview/diffrender"
	"github.com/elentok/gx/ui/search"
)

type visibleDiffRow struct {
	DisplayIndex       int
	RawIndex           int
	Text               string
	Kind               diffrender.RowKind
	InActiveHunk       bool
	IsActiveRaw        bool
	IsActiveChangedRaw bool
	OverflowTop        bool
	OverflowBottom     bool
	IsSeparator        bool
}

// RenderOpts controls how RenderRows assembles each visible diff line.
type RenderOpts struct {
	AccentColor color.Color
	InnerWidth  int
	SearchMatch func(displayIdx int) (matched, current bool)
	SearchQuery string
}

// RenderRows returns bodyH fully-assembled diff lines ready to embed in a panel
// frame. Each line is: mark (2 chars) + body (InnerWidth-2 chars, padded).
// Lines past the end of content are returned as empty strings.
// When search input is active, the last few lines are replaced with the search
// box so the total length is always exactly bodyH.
func (m *Model) RenderRows(bodyH int, active bool, opts RenderOpts) []string {
	var searchLines []string
	diffBodyH := bodyH
	if m.search.InputFocused() {
		m.search.SetWidth(m.viewport.Width())
		searchLines = strings.Split(m.search.View(), "\n")
		diffBodyH = maxInt(0, bodyH-len(searchLines))
	}

	rows := m.visibleRows(diffBodyH, active)
	const markW = 2
	const scrollGutterW = 2
	bodyW := maxInt(0, opts.InnerWidth-markW-scrollGutterW)
	overTop, overBottom, overBoth := m.overflowMarkers()

	lines := make([]string, 0, bodyH)
	for _, row := range rows {
		if row.DisplayIndex < 0 || row.DisplayIndex >= len(m.data.ViewLines) {
			lines = append(lines, "")
			continue
		}
		displayIdx := row.DisplayIndex

		// 1. Mark selection
		mark := "  "
		if opts.AccentColor != nil {
			ac := opts.AccentColor
			if m.navMode == NavModeLine && m.data.VisualActive && m.isDisplayInVisualRange(displayIdx) {
				mark = lipgloss.NewStyle().Foreground(ac).Render("▎ ")
			}
			if row.InActiveHunk && active {
				mark = lipgloss.NewStyle().Foreground(ac).Render("▌ ")
			}
			if row.IsActiveRaw {
				mark = lipgloss.NewStyle().Foreground(ac).Bold(true).Render("▌ ")
			}
			if row.IsActiveChangedRaw {
				mark = lipgloss.NewStyle().Foreground(ac).Bold(true).Render("▌ ")
			}
			if row.InActiveHunk {
				if row.OverflowTop && row.OverflowBottom {
					mark = lipgloss.NewStyle().Foreground(ac).Bold(true).Render(overBoth)
				} else if row.OverflowTop {
					mark = lipgloss.NewStyle().Foreground(ac).Bold(true).Render(overTop)
				} else if row.OverflowBottom {
					mark = lipgloss.NewStyle().Foreground(ac).Bold(true).Render(overBottom)
				}
			}
		}

		// 2. Body truncation
		body := ansi.Truncate(row.Text, bodyW, "")

		// 3. Separator dimming
		if row.IsSeparator {
			body = ui.StyleDiffSeparator.Render(ansi.Strip(body))
		}

		// 4. Search highlighting
		if opts.SearchMatch != nil {
			if matched, current := opts.SearchMatch(displayIdx); matched {
				body = search.Highlight(ansi.Strip(body), opts.SearchQuery, current)
			}
		}

		// 5. Padding
		body += diffrender.DiffBodyPadding(row.Kind, maxInt(0, bodyW-ansi.StringWidth(body)))

		// 6. Assembly (no trailing indicator)
		lines = append(lines, mark+body)
	}

	for len(lines) < diffBodyH {
		lines = append(lines, "")
	}
	lines = appendScrollbar(lines, markW+bodyW, diffBodyH, len(m.data.ViewLines), diffBodyH, m.viewport.YOffset())
	lines = append(lines, searchLines...)
	return lines
}

// appendScrollbar right-aligns a 2-column gutter (" " + glyph) onto each of
// the height content rows, padding shorter rows (e.g. blank rows past the end
// of content) out to padW first so every glyph lands in the same column,
// mirroring filetree's scrollbar gutter so diff panes get the same visual
// scroll indicator. The gutter renders blank when the content fits without
// scrolling.
func appendScrollbar(lines []string, padW, height, total, visible, offset int) []string {
	bar := ui.RenderScrollbar(height, total, visible, offset)
	var barLines []string
	if bar != "" {
		barLines = strings.Split(bar, "\n")
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		b := " "
		if i < len(barLines) {
			b = barLines[i]
		}
		pad := maxInt(0, padW-ansi.StringWidth(line))
		out[i] = line + strings.Repeat(" ", pad) + " " + b
	}
	return out
}

func (m *Model) overflowMarkers() (top, bottom, both string) {
	if m.useNerdFontIcons {
		return "\xef\x81\xa2 ", "\xef\x81\xa3 ", "↕ "
	}
	return "↑ ", "↓ ", "↕ "
}

func (m *Model) isDisplayInVisualRange(displayIdx int) bool {
	if !m.data.VisualActive || m.navMode != NavModeLine {
		return false
	}
	start, end := m.data.VisualLineBounds()
	if len(m.data.ChangedDisplay) > 0 {
		// Side-by-side mode: use ChangedDisplay mapping.
		for i := start; i <= end && i < len(m.data.ChangedDisplay); i++ {
			if i >= 0 && m.data.ChangedDisplay[i] == displayIdx {
				return true
			}
		}
		return false
	}
	// Unified mode: map displayIdx back to a raw index via DisplayToRaw,
	// then check if that raw line is one of the changed lines in [start, end].
	if displayIdx < 0 || displayIdx >= len(m.data.DisplayToRaw) {
		return false
	}
	rawIdx := m.data.DisplayToRaw[displayIdx]
	if rawIdx < 0 {
		return false
	}
	for i := start; i <= end && i < len(m.data.Parsed.Changed); i++ {
		if i >= 0 && m.data.Parsed.Changed[i].LineIndex == rawIdx {
			return true
		}
	}
	return false
}

func isSeparatorRow(text string, renderMode RenderMode) bool {
	if renderMode != RenderModeSideBySide {
		return false
	}
	return IsDeltaSectionDivider(strings.TrimSpace(ansi.Strip(text)))
}
