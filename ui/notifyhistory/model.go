// Package notifyhistory is an embeddable app-level modal that displays the
// entries notifylog captured, with /-search filtering (ui/search's existing
// contract) and a ww chord that exports the visible entries to disk.
package notifyhistory

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/notifylog"
	"github.com/elentok/gx/ui/search"
)

type Model struct {
	IsOpen bool

	entries      []notifylog.Entry
	repoName     string
	worktreeName string

	search   search.Model
	scroll   int
	pendingW bool // mid-"ww" chord
}

func New() Model {
	return Model{}
}

// Open shows the modal over entries (oldest first, as notifylog.Log.Entries
// returns them). repoName/worktreeName are embedded in the ww export
// filename.
func (m Model) Open(entries []notifylog.Entry, repoName, worktreeName string) Model {
	m.IsOpen = true
	m.entries = entries
	m.repoName = repoName
	m.worktreeName = worktreeName
	m.search = search.NewModel()
	m.scroll = 0
	m.pendingW = false
	return m
}

func (m Model) Close() Model {
	m.IsOpen = false
	return m
}

// Result reports what happened during Update, since the app shell needs to
// know when to stop routing keys/mice here (Closed) but Model itself always
// carries the authoritative IsOpen state.
type Result struct {
	Closed bool
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		return m.updateKey(key)
	}
	return m, nil, Result{}
}

func (m Model) updateKey(msg tea.KeyPressMsg) (Model, tea.Cmd, Result) {
	if nextSearch, cmd, result := m.search.Update(msg); result.Handled {
		m.search = nextSearch
		if result.QueryChanged {
			m.recomputeSearchMatches()
			m.scroll = 0
		}
		return m, cmd, Result{}
	}
	if m.search.InputFocused() {
		return m, nil, Result{}
	}

	key := msg.String()
	if m.pendingW {
		m.pendingW = false
		if key == "w" {
			return m.export()
		}
		return m, nil, Result{}
	}

	switch key {
	case "esc", "q":
		return m.Close(), nil, Result{Closed: true}
	case "w":
		m.pendingW = true
		return m, nil, Result{}
	case "j", "down":
		// Upper-bounded against the visible body height at render time
		// (renderFrame), since screen size isn't known here.
		m.scroll = min(m.scroll+1, len(m.visibleEntries()))
		return m, nil, Result{}
	case "k", "up":
		m.scroll = max(m.scroll-1, 0)
		return m, nil, Result{}
	case "g":
		m.scroll = 0
		return m, nil, Result{}
	case "G":
		m.scroll = len(m.visibleEntries())
		return m, nil, Result{}
	}
	return m, nil, Result{}
}

func clampScroll(offset, total, visible int) int {
	maxOffset := max(total-visible, 0)
	return max(0, min(offset, maxOffset))
}

func (m *Model) recomputeSearchMatches() {
	q := strings.ToLower(strings.TrimSpace(m.search.Query()))
	if q == "" {
		m.search.SetMatches(nil)
		return
	}
	matches := make([]search.Match, 0)
	for i, e := range m.entries {
		if strings.Contains(strings.ToLower(entryText(e)), q) {
			matches = append(matches, search.Match{DataIndex: i})
		}
	}
	m.search.SetMatches(matches)
}

// visibleEntries returns the rows currently shown: everything, or - once a
// search query is active - only the ui/search matches, in original order.
// This is also what ww exports, so search and export always agree on what
// "visible" means.
func (m Model) visibleEntries() []notifylog.Entry {
	if !m.search.HasQuery() {
		return m.entries
	}
	matches := m.search.Matches()
	visible := make([]notifylog.Entry, 0, len(matches))
	for _, match := range matches {
		if match.DataIndex >= 0 && match.DataIndex < len(m.entries) {
			visible = append(visible, m.entries[match.DataIndex])
		}
	}
	return visible
}

func kindLabel(kind notify.NotifyKind) string {
	switch kind {
	case notify.KindSuccess:
		return "success"
	case notify.KindWarning:
		return "warning"
	case notify.KindError:
		return "error"
	case notify.KindProgress:
		return "progress"
	default:
		return "info"
	}
}

func entryText(e notifylog.Entry) string {
	status := ""
	if e.Kind == notify.KindProgress && e.Closed {
		status = " closed"
	}
	return fmt.Sprintf("%s %s%s %s", e.Time.Format(time.TimeOnly), kindLabel(e.Kind), status, e.Message)
}
