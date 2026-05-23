package nav

import tea "charm.land/bubbletea/v2"

// AppendViewStateChanged appends a ViewStateChanged command when navigation is enabled
// and the view state changed between pre/post update.
func AppendViewStateChanged(cmd tea.Cmd, enabled bool, prev ViewState, prevOK bool, next ViewState, nextOK bool) tea.Cmd {
	if !enabled || !nextOK {
		return cmd
	}
	if !prevOK || next != prev {
		return tea.Batch(cmd, ViewStateChanged(next))
	}
	return cmd
}
