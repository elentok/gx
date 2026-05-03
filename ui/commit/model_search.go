package commit

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/explorer"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type commitSearchMode int

const (
	searchModeNone commitSearchMode = iota
	searchModeInput
)

var commitSearchHighlightStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)
var commitSearchCurrentStyle = lipgloss.NewStyle().Foreground(ui.ColorGreen).Bold(true)

func (m *Model) enterSearchMode() {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.SetValue(m.searchQuery)
	ti.CursorEnd()
	ti.Focus()
	m.searchInput = ti
	m.searchMode = searchModeInput
}

func (m *Model) handleSearchKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.searchMode != searchModeInput {
		return false, nil
	}
	switch msg.String() {
	case "esc":
		m.searchMode = searchModeNone
		if strings.TrimSpace(m.searchQuery) == "" || len(m.searchMatches) == 0 {
			m.clearSearch()
		}
		return true, nil
	case "enter":
		m.searchMode = searchModeNone
		if strings.TrimSpace(m.searchQuery) == "" || len(m.searchMatches) == 0 {
			m.clearSearch()
		}
		return true, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.searchQuery = m.searchInput.Value()
	m.recomputeSearchMatches()
	m.jumpToSearchCursor()
	return true, cmd
}

func (m *Model) handleSearchNavigateKey(msg tea.KeyPressMsg) bool {
	if strings.TrimSpace(m.searchQuery) == "" || len(m.searchMatches) == 0 {
		return false
	}
	switch msg.String() {
	case "n":
		if m.searchCursor < len(m.searchMatches)-1 {
			m.searchCursor++
		}
		m.jumpToSearchCursor()
		return true
	case "N", "shift+n":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		m.jumpToSearchCursor()
		return true
	}
	return false
}

func (m *Model) clearSearch() {
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchCursor = 0
}

func (m *Model) recomputeSearchMatches() {
	m.searchMatches = nil
	m.searchCursor = 0
	m.searchMatches = explorer.ComputeDiffSearchMatches(m.section.ViewLines, m.section.DisplayToRaw, m.searchQuery)
}

func (m *Model) jumpToSearchCursor() {
	if len(m.searchMatches) == 0 || m.searchCursor < 0 || m.searchCursor >= len(m.searchMatches) {
		return
	}
	match := m.searchMatches[m.searchCursor]
	m.focusDiff = true
	m.diffNavMode = explorer.NavLine
	explorer.ApplyDiffSearchMatch(&m.section, &m.diffViewport, match)
}

func (m Model) searchMatchDiffDisplay(displayIdx int) (matched bool, current bool) {
	if strings.TrimSpace(m.searchQuery) == "" {
		return false, false
	}
	if i := explorer.DiffSearchMatchIndex(m.searchMatches, displayIdx); i >= 0 {
		return true, i == m.searchCursor
	}
	return false, false
}

func highlightMatchText(text, query string, current bool) string {
	if strings.TrimSpace(query) == "" {
		return text
	}
	lower := strings.ToLower(text)
	lq := strings.ToLower(query)
	idx := strings.Index(lower, lq)
	if idx < 0 {
		return text
	}
	end := idx + len(query)
	if end > len(text) {
		end = len(text)
	}
	style := commitSearchHighlightStyle
	if current {
		style = commitSearchCurrentStyle
	}
	return text[:idx] + style.Render(text[idx:end]) + text[end:]
}

func (m Model) searchFooterText() string {
	if m.searchMode != searchModeInput {
		return ""
	}
	total := len(m.searchMatches)
	right := ""
	if strings.TrimSpace(m.searchQuery) != "" {
		if total == 0 {
			right = "no matches"
		} else {
			right = fmt.Sprintf("%d/%d", m.searchCursor+1, total)
		}
	}
	left := m.searchInput.View()
	if right == "" || m.width <= 0 {
		return left
	}
	leftW := ansi.StringWidth(left)
	rightW := ansi.StringWidth(right)
	if leftW+rightW+1 >= m.width {
		return left + " " + right
	}
	return left + strings.Repeat(" ", m.width-leftW-rightW) + right
}
