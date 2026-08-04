package tickets

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ui"
)

// metricsLineIndent aligns a row's second line under its first line's title
// text (after the "  " + icon + " " lead-in), the same indentation width
// regardless of icon glyph.
const metricsLineIndent = "    "

// metricsLineStyle renders a live row's second line dim+italic, distinguishing
// it from the title line above it. Disk-status rows instead color their
// second line to match the first line's status style (ticket 02) — see
// renderRowMetricsLine.
var metricsLineStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtleLight).Italic(true)

// formatTokenCount abbreviates tokens for a row's metrics line: plain below
// 1000, "45.2k tok" below 1,000,000, "1.2M tok" at or above.
func formatTokenCount(tokens int) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.1fM tok", float64(tokens)/1_000_000)
	case tokens >= 1000:
		return fmt.Sprintf("%.1fk tok", float64(tokens)/1000)
	default:
		return fmt.Sprintf("%d tok", tokens)
	}
}

// formatElapsed renders seconds as "12m34s"/"1h05m" — no space between value
// and unit, seconds dropped past an hour. Below a full minute it renders as
// a bare "Ns" (e.g. "0s"), rather than "0m00s", so the 0/0 "never stamped"
// sentinel still reads cleanly.
func formatElapsed(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%02dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// formatMetricsLine joins elapsed/tokens into a row's line-2 figures, e.g.
// "12m34s · 45.2k tok".
func formatMetricsLine(elapsedSeconds, tokens int) string {
	return formatElapsed(elapsedSeconds) + " · " + formatTokenCount(tokens)
}

// joinNonEmpty joins a and b with sep, skipping either side if empty — a
// live row's line-2 composition, where a live row's suffix (phase/label or
// pause reason) is sometimes absent.
func joinNonEmpty(sep, a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + sep + b
	}
}

// renderMetricsLine renders text (a row's line-2 content, already composed
// from whatever suffix/reason/figures apply) indented and italicized under
// its title line, in style's foreground — the same status style as the row's
// first-line icon/text, so the two lines read as one visual unit (ticket 02).
func renderMetricsLine(text string, style lipgloss.Style) string {
	return metricsLineIndent + style.Italic(true).Render(text)
}

// renderRowMetricsLine indents text two columns deeper than renderMetricsLine
// alone, aligning a row's line-2 under its title text after the "  " + icon
// lead-in — shared by the Tickets tab's tree rows (Model.renderTicketRow) and
// the Queue tab's flat rows (renderQueueTicketRow) so both read identically.
func renderRowMetricsLine(text string, style lipgloss.Style) string {
	return "  " + renderMetricsLine(text, style)
}
