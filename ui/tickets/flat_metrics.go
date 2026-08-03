package tickets

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ui"
)

// metricsLineIndent aligns a row's second line under its first line's title
// text (after the "  " + icon + " " lead-in), the same indentation width
// regardless of icon glyph.
const metricsLineIndent = "    "

// metricsLineStyle renders a row's second line dim+italic, distinguishing it
// from the title line above it.
var metricsLineStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)

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

// liveElapsedSeconds computes a running/paused ticket's elapsed time from
// live.startedAt (the session transcript's first-line timestamp), so it
// keeps climbing across renders without a UI-side stopwatch. Zero if
// startedAt hasn't resolved yet (see resolveStartedAt).
func liveElapsedSeconds(live liveTicketState) int {
	if live.startedAt.IsZero() {
		return 0
	}
	return int(time.Since(live.startedAt).Seconds())
}

// joinNonEmpty joins a and b with sep, skipping either side if empty —
// renderFlatTicketRow's line-2 composition, where a live row's suffix
// (phase/label or pause reason) is sometimes absent.
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
// from whatever suffix/reason/figures apply) indented and styled under its
// title line.
func renderMetricsLine(text string) string {
	return metricsLineIndent + metricsLineStyle.Render(text)
}

// landedSegmentText renders ticket 03's landed-check outcome for a done
// ticket's metrics line third segment: "landed" (ok, present — the expected
// case) plain, "⚠ not landed" (ok, absent — a genuine anomaly) flagged as
// warn so the caller can pop it in the existing attention style, and
// "landed?" (check unavailable) dim like the rest of the line, since an
// inconclusive check is not itself an anomaly.
func landedSegmentText(icons ui.IconSet, ok, present bool) (text string, warn bool) {
	switch {
	case !ok:
		return "landed?", false
	case present:
		return "landed", false
	default:
		return icons.Warning + " not landed", true
	}
}

// renderMetricsLineWithLanded appends landed's segment to base (already
// composed elapsed/token figures), keeping the middle-dot separator in the
// line's normal dim/italic style even when the segment itself pops in the
// warning style.
func renderMetricsLineWithLanded(base string, icons ui.IconSet, ok, present bool) string {
	text, warn := landedSegmentText(icons, ok, present)
	if !warn {
		return renderMetricsLine(base + " · " + text)
	}
	return metricsLineIndent + metricsLineStyle.Render(base+" · ") + statusNeedsAttentionStyle.Render(text)
}
