package search

import tea "charm.land/bubbletea/v2"

type SearchQueryUpdatedMsg struct {
	Query string
}

type JumpToMatchMsg struct {
	Match Match
}

func createSearchQueryUpdatedCmd(query string) tea.Cmd {
	return func() tea.Msg {
		return SearchQueryUpdatedMsg{Query: query}
	}
}

func jumpToMatchCmd(match Match) tea.Cmd {
	return func() tea.Msg {
		return JumpToMatchMsg{Match: match}
	}
}
