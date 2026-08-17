package tickets

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

// metricsLineStyle renders a live row's inline elapsed/token suffix
// dim+italic, distinguishing it from the title text before it. Disk-status
// rows instead color that suffix to match the first line's status style
// (ticket 02).
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

// formatDuration renders a wall-clock span (epic.yaml's started_at ->
// completed_at) as "Xh Ym"/"Xd Yh"/"Xm" — distinct from formatElapsed's
// per-ticket "12m34s" seconds precision, since a duration spanning idle time
// between ticket runs has no meaningful seconds component. Days roll over
// past 24h and hours roll over past 60m so each unit stays natural to read
// (no "26h05m", no "0h05m").
func formatDuration(d time.Duration) string {
	totalMinutes := max(int(d.Round(time.Minute).Minutes()), 0)
	days := totalMinutes / (24 * 60)
	hours := (totalMinutes / 60) % 24
	minutes := totalMinutes % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// formatMetricsLine joins elapsed/tokens/cost into a row's line-2 figures,
// e.g. "12m34s · 45.2k tok · $0.42". Each figure is omitted independently
// when zero — a live row's cost isn't known until the iteration lands, and a
// ticket that only recorded one or two of the three fields (or none at all)
// shouldn't render a misleading "0s"/"0 tok" for the ones it never landed.
func formatMetricsLine(elapsedSeconds, tokens int, cost float64) string {
	var parts []string
	if elapsedSeconds > 0 {
		parts = append(parts, formatElapsed(elapsedSeconds))
	}
	if tokens > 0 {
		parts = append(parts, formatTokenCount(tokens))
	}
	if cost > 0 {
		parts = append(parts, tickets.FormatCost(cost))
	}
	return strings.Join(parts, " · ")
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

// appendRowMetrics appends text (a row's elapsed/token or live suffix
// content, already composed from whatever suffix/reason/figures apply) to
// the end of line, separated by a space and italicized in style's
// foreground — shared by the Tickets tab's tree rows (Model.renderTicketRow)
// and the Queue tab's flat rows (renderQueueTicketRow) so both read
// identically (ticket 06 folded this onto the title line itself).
func appendRowMetrics(line, text string, style lipgloss.Style) string {
	return line + " " + style.Italic(true).Render(text)
}
