package search

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/keys"
)

type SearchMode int

const (
	SearchModeNone SearchMode = iota
	SearchModeInput
	SearchModeResults
)

type Match struct {
	DataIndex   int // filetree entry index or diff raw line index
	ViewportRow int // index into rendered viewport lines
}

type Model struct {
	textinput textinput.Model
	mode      SearchMode
	cursor    int
	query     string
	matches   []Match

	viewportRowToPos map[int]int
	dataIndexToPos   map[int]int

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

func (m *Model) InputFocused() bool {
	return m.mode == SearchModeInput
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

func (m Model) Keys() keys.Manager {
	return keys.New([]keys.Binding{
		{Seq: []string{"/"}, Categories: []string{"Search"}, Title: "search"},
		{Seq: []string{"n"}, Categories: []string{"Search"}, Title: "next result"},
		{Seq: []string{"N"}, Categories: []string{"Search"}, Title: "prev result"},
	})
}

func (m *Model) Start(initialQuery string) {
	m.textinput.SetValue(initialQuery)
	m.query = initialQuery
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
	m.viewportRowToPos = nil
	m.dataIndexToPos = nil
}

func (m *Model) SetWidth(width int) {
	m.width = width
	// 2 columns for the frame + 2 columns for padding
	m.textinput.SetWidth(width - 4)
}

func (m *Model) MatchPosByViewportRow(row int) (int, bool) {
	pos, ok := m.viewportRowToPos[row]
	return pos, ok
}

func (m *Model) MatchPosByDataIndex(idx int) (int, bool) {
	pos, ok := m.dataIndexToPos[idx]
	return pos, ok
}

func (m *Model) SetMatches(matches []Match) {
	m.matches = matches

	if len(m.matches) > 0 {
		if m.cursor < 0 || m.cursor >= len(m.matches) {
			m.cursor = 0
		}
	}

	m.viewportRowToPos = make(map[int]int, len(matches))
	m.dataIndexToPos = make(map[int]int, len(matches))
	for i, match := range matches {
		m.viewportRowToPos[match.ViewportRow] = i
		m.dataIndexToPos[match.DataIndex] = i
	}
}

// SetPassiveResults stores query + matches while leaving search inactive.
// This preserves highlight/navigation context without consuming esc/q.
func (m *Model) SetPassiveResults(query string, matches []Match) {
	m.textinput.SetValue(query)
	m.query = query
	m.mode = SearchModeNone
	m.SetMatches(matches)
}

func (m *Model) SetMatchesAndJump(matches []Match) tea.Cmd {
	m.SetMatches(matches)
	if match, ok := m.Match(m.cursor); ok {
		return jumpToMatchCmd(match)
	}

	return nil
}
