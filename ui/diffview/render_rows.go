package diffview

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui/diffview/diffrender"
	"github.com/elentok/gx/ui/search"
)

type VisibleDiffRow struct {
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

// diffSeparatorStyle dims delta section-divider lines in side-by-side mode.
// Matches ui.StyleDiffSeparator without importing the ui package.
var diffSeparatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#11111a"))

// RenderRows returns bodyH fully-assembled diff lines ready to embed in a panel
// frame. Each line is: mark (2 chars) + body (InnerWidth-2 chars, padded).
// Lines past the end of content are returned as empty strings.
func (m *Model) RenderRows(bodyH int, active bool, opts RenderOpts) []string {
	rows := m.VisibleRows(bodyH, active)
	const markW = 2
	bodyW := maxInt(0, opts.InnerWidth-markW)
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
			body = diffSeparatorStyle.Render(ansi.Strip(body))
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

	for len(lines) < bodyH {
		lines = append(lines, "")
	}
	return lines
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
	if len(m.data.ChangedDisplay) == 0 {
		return false
	}
	start, end := m.data.VisualLineBounds()
	for i := start; i <= end && i < len(m.data.ChangedDisplay); i++ {
		if i >= 0 && m.data.ChangedDisplay[i] == displayIdx {
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
