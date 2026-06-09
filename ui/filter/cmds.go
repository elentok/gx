package filter

import tea "charm.land/bubbletea/v2"

// FilterChangedMsg is emitted whenever the query text changes so a host that
// prefers a message-driven recompute can react. Hosts that own the filter inline
// (like ui/help) may instead read Result.QueryChanged and recompute synchronously.
type FilterChangedMsg struct {
	Query string
}

func createFilterChangedCmd(query string) tea.Cmd {
	return func() tea.Msg {
		return FilterChangedMsg{Query: query}
	}
}
