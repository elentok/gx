package search

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type SearchMode int

const (
	SearchModeNone SearchMode = iota
	SearchModeInput
	SearchModeResults
)

type Match struct {
	Index        int
	DisplayIndex int
}

type Model struct {
	textinput textinput.Model
	mode      SearchMode
	cursor    int
	query     string
	matches   []Match

	width int
}

func (m *Model) Query() string {
	return m.query
}

func (m *Model) HasQuery() bool {
	return strings.TrimSpace(m.query) != ""
}

func (m *Model) Cursor() int {
	return m.cursor
}

func (m *Model) SetCursor(newValue int) {
	m.cursor = newValue
}

func (m *Model) Mode() SearchMode {
	return m.mode
}

func (m *Model) IsActive() bool {
	return m.mode != SearchModeNone
}

func (m *Model) MatchesCount() int {
	return len(m.matches)
}

func (m Model) Matches() []Match {
	return m.matches
}

func (m Model) Match(index int) (Match, bool) {
	if index < 0 || index >= len(m.matches) {
		return Match{}, false
	}

	return m.matches[index], true
}

func NewModel() Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CursorEnd()
	ti.Focus()

	return Model{
		textinput: ti,
		matches:   []Match{},
		width:     50,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Start(initialQuery string) {
	m.textinput.SetValue(initialQuery)
	m.mode = SearchModeInput
}

func (m *Model) DismissAndClear() {
	m.clear()
	m.mode = SearchModeNone
}

func (m *Model) DismissAndKeepResults() {
	if !m.HasQuery() || len(m.matches) == 0 {
		m.clear()
		m.mode = SearchModeNone
	} else {
		m.mode = SearchModeResults
	}
}

func (m *Model) clear() {
	m.query = ""
	m.cursor = 0
	m.matches = []Match{}
}

func (m *Model) SetWidth(width int) {
	m.width = width
	// 2 columns for the frame + 2 columns for padding
	m.textinput.SetWidth(width - 4)
}

func (m *Model) SetMatches(matches []Match) {
	m.matches = matches

	if len(m.matches) > 0 {
		if m.cursor < 0 || m.cursor >= len(m.matches) {
			m.cursor = 0
		}
	}
}

func (m *Model) SetMatchesAndJump(matches []Match) tea.Cmd {
	m.SetMatches(matches)
	if match, ok := m.Match(m.cursor); ok {
		return jumpToMatchCmd(match)
	}

	return nil
}
