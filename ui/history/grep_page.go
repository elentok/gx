package history

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/filter"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/search"
)

// enterGrep opens the grep page from either the projects or conversations
// page (ctrl+f), remembering which so esc can return to it. Search scope
// defaults to the originating project directory when opened from
// conversations, global otherwise, mirroring blf's enterGrep.
func (m Model) enterGrep() (tea.Model, tea.Cmd) {
	m.grepFromPage = m.page
	m.page = pageGrep
	m.grepResults = nil
	m.grepErr = nil
	m.rgNotFound = false
	m.grepRunning = false
	m.grepList = list.Model{}
	m.grepFilter = filter.NewModel()
	m.grepFilter.SetWidth(m.w - 4)
	m.grepFilter.Start()

	if m.grepFromPage == pageConversations && m.convProjectDir != "" {
		m.grepScope = grepScopeProject
		m.grepScopeProjDir = m.convProjectDir
	} else {
		m.grepScope = grepScopeGlobal
	}
	return m, nil
}

func (m Model) exitGrep() (tea.Model, tea.Cmd) {
	m.page = m.grepFromPage
	return m, nil
}

func (m Model) handleGrepKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Reserved control bindings are intercepted before the filter sees them,
	// since the filter is always focused on this page (Start() on entry, no
	// "/" needed) and would otherwise swallow every keypress as typed text.
	// Navigation therefore uses the arrow keys only (not j/k, which are
	// ordinary query characters here).
	switch msg.String() {
	case "esc":
		return m.exitGrep()
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+g":
		return m.toggleGrepScope()
	case "enter":
		return m, m.cmdExportAndEditGrep()
	case "ctrl+r":
		return m, m.cmdResumeGrepResult()
	case "ctrl+y":
		return m, m.cmdYankGrepSessionID()
	case "down":
		m.grepList.Navigate(1, len(m.grepResults), m.listHeight())
		return m, nil
	case "up":
		m.grepList.Navigate(-1, len(m.grepResults), m.listHeight())
		return m, nil
	case "ctrl+d":
		m.grepList.ScrollPage(list.DefaultScroll, len(m.grepResults), m.listHeight())
		return m, nil
	case "ctrl+u":
		m.grepList.ScrollPage(-list.DefaultScroll, len(m.grepResults), m.listHeight())
		return m, nil
	}

	if m.grepFilter.InputFocused() {
		var res filter.Result
		var filterCmd tea.Cmd
		m.grepFilter, filterCmd, res = m.grepFilter.Update(msg)
		if res.QueryChanged {
			return m, tea.Batch(filterCmd, m.startGrepDebounce())
		}
		if res.Handled {
			return m, filterCmd
		}
	}
	return m, nil
}

// toggleGrepScope flips between searching only the originating project
// directory and every known project directory, then re-runs the current
// query under the new scope.
func (m Model) toggleGrepScope() (tea.Model, tea.Cmd) {
	if m.grepScope == grepScopeProject {
		m.grepScope = grepScopeGlobal
	} else if m.grepScopeProjDir != "" {
		m.grepScope = grepScopeProject
	}
	m.grepList.SetSelected(0, 1)
	return m, m.startGrepDebounce()
}

// grepDirs returns the directories the current scope searches.
func (m Model) grepDirs() []string {
	if m.grepScope == grepScopeProject && m.grepScopeProjDir != "" {
		return []string{m.grepScopeProjDir}
	}
	dirs := make([]string, len(m.projects))
	for i, p := range m.projects {
		dirs[i] = p.Dir
	}
	return dirs
}

// startGrepDebounce schedules a grepDebounceMsg grepDebounce from now,
// stamped with a bumped sequence number so a stale debounce (superseded by a
// later query change) is discarded on arrival.
func (m *Model) startGrepDebounce() tea.Cmd {
	m.grepSeq++
	seq := m.grepSeq
	query := m.grepFilter.Query()
	return tea.Tick(grepDebounce, func(time.Time) tea.Msg {
		return grepDebounceMsg{seq: seq, query: query}
	})
}

func (m Model) handleGrepDebounce(msg grepDebounceMsg, notifyCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if msg.seq != m.grepSeq || msg.query != m.grepFilter.Query() {
		return m, notifyCmd // stale: superseded by a later query change
	}
	if msg.query == "" {
		m.grepResults = nil
		m.grepRunning = false
		m.grepList.SetSelected(0, 1)
		return m, notifyCmd
	}
	m.grepRunning = true
	seq := m.grepSeq
	query := msg.query
	dirs := m.grepDirs()
	grepFunc := m.grepFunc
	return m, tea.Batch(notifyCmd, func() tea.Msg {
		results, err := grepFunc(query, dirs)
		return grepResultsMsg{results: results, err: err, seq: seq}
	})
}

func (m Model) handleGrepResults(msg grepResultsMsg, notifyCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if msg.seq != m.grepSeq {
		return m, notifyCmd // stale: a newer search is already in flight
	}
	m.grepRunning = false
	if msg.err != nil {
		if errors.Is(msg.err, claudehistory.ErrRgNotFound) {
			m.rgNotFound = true
		} else {
			m.grepErr = msg.err
		}
		return m, notifyCmd
	}
	m.grepErr = nil
	m.rgNotFound = false
	m.grepResults = msg.results
	m.grepList.SetSelected(0, max(len(msg.results), 1))
	return m, notifyCmd
}

func (m Model) viewGrep() string {
	if m.rgNotFound {
		return ui.StyleWarning.Render(claudehistory.ErrRgNotFound.Error())
	}

	items := m.grepResults
	sel := m.grepList.Selected()
	start, end := m.grepList.VisibleRange(len(items), m.listHeight())
	var lines []string
	for i := start; i < end; i++ {
		line := renderGrepRow(items[i], m.grepFilter.Query())
		if i == sel {
			line = ui.RenderRowHighlight(renderGrepRowPlain(items[i]))
		}
		lines = append(lines, line)
	}
	if m.grepErr != nil {
		lines = []string{ui.StyleWarning.Render("error: " + m.grepErr.Error())}
	} else if len(lines) == 0 {
		lines = []string{ui.StyleMuted.Render(grepEmptyMessage(m))}
	}

	scopeLabel := "global"
	if m.grepScope == grepScopeProject {
		scopeLabel = "project"
	}
	panel := ui.RenderPanel(ui.PanelOptionsFor(m.w, m.h-2-m.grepPreviewBoxHeight(), "Grep Transcripts", "scope: "+scopeLabel, lines, true, ui.ColorOrange, ui.ColorOrange, false))
	top := m.grepFilter.View() + "\n"
	footer := "  " + ui.StyleHint.Render("enter: export+edit  ctrl+r: resume  ctrl+y: yank id  ctrl+g: toggle scope  esc: back")
	return top + panel + "\n" + m.viewGrepPreview() + "\n" + footer
}

func grepEmptyMessage(m Model) string {
	if m.grepFilter.Query() == "" {
		return "type to search across transcripts"
	}
	if m.grepRunning {
		return "searching…"
	}
	return "no results"
}

func renderGrepRow(r claudehistory.GrepResult, query string) string {
	return renderGrepRowText(r, false, query)
}

func renderGrepRowPlain(r claudehistory.GrepResult) string {
	return renderGrepRowText(r, true, "")
}

func renderGrepRowText(r claudehistory.GrepResult, selected bool, query string) string {
	title := r.ConvTitle
	if title == "" {
		title = r.SessionID
	}
	snippet := r.Snippet
	if query != "" {
		snippet = search.Highlight(snippet, query, selected)
	}
	return title + "  " + ui.StyleMuted.Render(snippet)
}

// viewGrepPreview renders the bordered preview box below the results list,
// showing the selected result's full preview text, conversation title, and
// session id — sized to ~1/3 of terminal height (grepPreviewBoxHeight),
// clamped like blf's original.
func (m Model) viewGrepPreview() string {
	boxHeight := max(m.grepPreviewBoxHeight(), 3)
	sel := m.grepList.Selected()

	var lines []string
	if len(m.grepResults) == 0 || sel >= len(m.grepResults) {
		lines = []string{ui.StyleMuted.Render("no result selected")}
	} else {
		r := m.grepResults[sel]
		if r.ConvTitle != "" {
			lines = append(lines, ui.StyleMuted.Render("conversation: "+r.ConvTitle))
		}
		if r.SessionID != "" {
			lines = append(lines, ui.StyleMuted.Render("session: "+r.SessionID))
		}
		text := r.Preview
		if text == "" {
			text = r.Snippet
		}
		lines = append(lines, wrapLines(text, max(m.w-4, 20))...)
	}

	return ui.RenderPanel(ui.PanelOptionsFor(m.w, boxHeight, "Preview", "", lines, false, ui.ColorOrange, ui.ColorOrange, false))
}

// wrapLines wraps text to at most width runes per line (naive word wrap),
// splitting on existing newlines first.
func wrapLines(text string, width int) []string {
	if width <= 0 {
		width = 80
	}
	var lines []string
	para := []rune("")
	for _, r := range text {
		if r == '\n' {
			lines = append(lines, string(para))
			para = []rune("")
			continue
		}
		para = append(para, r)
	}
	lines = append(lines, string(para))

	var wrapped []string
	for _, line := range lines {
		runes := []rune(line)
		if len(runes) == 0 {
			wrapped = append(wrapped, "")
			continue
		}
		for len(runes) > width {
			wrapped = append(wrapped, string(runes[:width]))
			runes = runes[width:]
		}
		wrapped = append(wrapped, string(runes))
	}
	return wrapped
}
