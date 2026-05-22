package nav

import tea "charm.land/bubbletea/v2"

// AppendRouteChanged appends a RouteChanged command when navigation is enabled
// and the route identity changed between pre/post update.
func AppendRouteChanged(cmd tea.Cmd, enabled bool, prev Route, prevOK bool, next Route, nextOK bool) tea.Cmd {
	if !enabled || !nextOK {
		return cmd
	}
	if !prevOK || next != prev {
		return tea.Batch(cmd, RouteChanged(next))
	}
	return cmd
}
