package tickets

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
)

// bannerPausedStyle/bannerAttentionStyle render the emphasized banner line
// shown above the footer's plain "? help" line while the loop is paused or
// needs operator attention — deliberately louder than a row's own
// statusPausedStyle/statusNeedsAttentionStyle (view.go), since this is a
// loop-wide condition, not one ticket's badge.
var (
	bannerPausedStyle    = lipgloss.NewStyle().Bold(true).Foreground(ui.ColorMauve)
	bannerAttentionStyle = lipgloss.NewStyle().Bold(true).Foreground(ui.ColorOrange)
)

// bannerLine returns the paused/needs-attention banner text for the first
// live ticket found in each state, or "" if none of m.live is currently
// paused — which also covers a paused ticket having since resumed, since
// applyLiveEvent removes/overwrites its m.live entry on IterationResumed.
// A needs-attention reason wins if both are live at once, since it's the
// more actionable of the two.
func (m FlatModel) bannerLine() string {
	var pausedReason, attentionReason string
	for _, ls := range m.live {
		if !ls.paused {
			continue
		}
		if ls.pauseKind == ralphloop.PauseNeedsAttention {
			if attentionReason == "" {
				attentionReason = ls.reason
			}
			continue
		}
		if pausedReason == "" {
			pausedReason = ls.reason
		}
	}

	switch {
	case attentionReason != "":
		return bannerAttentionStyle.Render(fmt.Sprintf("Ralph loop needs attention — %s.", attentionReason))
	case pausedReason != "":
		return bannerPausedStyle.Render(fmt.Sprintf("Ralph loop paused — %s.", pausedReason))
	default:
		return ""
	}
}

// footerLineCount is how many lines footerView renders: the banner (0 or 1)
// plus the plain "? help" keyhints line (always 1).
func (m FlatModel) footerLineCount() int {
	if m.bannerLine() != "" {
		return 2
	}
	return 1
}

// footerView renders the bottom of the screen: the banner line, if either
// live state is present, stacked above the plain keyhints line every other
// ui tab uses (".ai/index.md": no keymaps on the statusbar, only "? help").
func (m FlatModel) footerView() string {
	help := "  " + ui.StyleHint.Render("? help")
	if banner := m.bannerLine(); banner != "" {
		return lipgloss.JoinVertical(lipgloss.Left, "  "+banner, help)
	}
	return help
}
