package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/terminalrun"
)

const (
	bindingTicketsBack           keys.BindingID = "back"
	bindingTicketsDown           keys.BindingID = "down"
	bindingTicketsUp             keys.BindingID = "up"
	bindingTicketsPageDown       keys.BindingID = "page-down"
	bindingTicketsPageUp         keys.BindingID = "page-up"
	bindingTicketsCollapse       keys.BindingID = "collapse"
	bindingTicketsExpand         keys.BindingID = "expand"
	bindingTicketsToggle         keys.BindingID = "toggle"
	bindingTicketsResume         keys.BindingID = "resume"
	bindingTicketsRefresh        keys.BindingID = "refresh"
	bindingTicketsEditInPlace    keys.BindingID = "edit"
	bindingTicketsEditHSplit     keys.BindingID = "edit-hsplit"
	bindingTicketsEditVSplit     keys.BindingID = "edit-vsplit"
	bindingTicketsEditTab        keys.BindingID = "edit-tab"
	bindingTicketsCancelChord    keys.BindingID = "cancel-chord"
	bindingTicketsImplement      keys.BindingID = "implement"
	bindingTicketsToggleCheck    keys.BindingID = "toggle-check"
	bindingTicketsToggleHideDone keys.BindingID = "toggle-hide-done"
	bindingTicketsSelectFirst    keys.BindingID = "select-first"
	bindingTicketsSelectLast     keys.BindingID = "select-last"
	bindingTicketsPreviewBottom  keys.BindingID = "preview-bottom"
)

// newTicketsManager builds the key manager for the sidebar's focus: plain
// up/down navigation plus collapse/expand bindings on an epic row (h/left
// collapse; l/right/enter expand a collapsed epic, or focus the
// preview panel if it's already expanded); on a ticket row, l/right/enter
// always hand focus to the preview panel (see focusPreviewOrExpand) —
// handlePreviewKey in model_preview_focus.go covers the preview panel's own
// bindings once focused, since only one panel's keys are live at a time.
func newTicketsManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingTicketsBack, Seq: []string{"q"}, Categories: []string{"Other"}, Title: "back"},
		{ID: bindingTicketsDown, Seq: []string{"j"}, Categories: []string{"Navigation"}, Title: "down", Display: "↓/j"},
		{ID: bindingTicketsDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsUp, Seq: []string{"k"}, Categories: []string{"Navigation"}, Title: "up", Display: "↑/k"},
		{ID: bindingTicketsUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsPageDown, Seq: []string{"ctrl+d"}, Categories: []string{"Navigation"}, Title: "page down"},
		{ID: bindingTicketsPageUp, Seq: []string{"ctrl+u"}, Categories: []string{"Navigation"}, Title: "page up"},
		{ID: bindingTicketsCollapse, Seq: []string{"h"}, Categories: []string{"Navigation"}, Title: "collapse epic", Display: "h/←"},
		{ID: bindingTicketsCollapse, Seq: []string{"left"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsExpand, Seq: []string{"l"}, Categories: []string{"Navigation"}, Title: "expand epic / focus preview", Display: "l/→"},
		{ID: bindingTicketsExpand, Seq: []string{"right"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsToggle, Seq: []string{"enter"}, Categories: []string{"Navigation"}, Title: "expand epic / focus preview"},
		{ID: bindingTicketsRefresh, Seq: []string{"R"}, Categories: []string{"Navigation"}, Title: "refresh"},
		// e-prefix chords: edit the selected row's underlying file, reusing
		// the same launch-mode plumbing every other tab's edit-chord uses.
		{ID: bindingTicketsEditInPlace, Seq: []string{"e", "e"}, Categories: []string{"Navigation"}, Title: "edit file"},
		{ID: bindingTicketsEditHSplit, Seq: []string{"e", "s"}, Categories: []string{"Navigation"}, Title: "edit file (split)"},
		{ID: bindingTicketsEditVSplit, Seq: []string{"e", "v"}, Categories: []string{"Navigation"}, Title: "edit file (vsplit)"},
		{ID: bindingTicketsEditTab, Seq: []string{"e", "t"}, Categories: []string{"Navigation"}, Title: "edit file (tab)"},
		{ID: bindingTicketsCancelChord, Seq: []string{"e", "esc"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsImplement, Seq: []string{"i"}, Categories: []string{"Navigation"}, Title: "implement epic"},
		{ID: bindingTicketsToggleCheck, Seq: []string{"space"}, Categories: []string{"Navigation"}, Title: "check/uncheck"},
		{ID: bindingTicketsToggleHideDone, Seq: []string{"t", "c"}, Categories: []string{"Navigation"}, Title: "hide completed"},
		{ID: bindingTicketsSelectFirst, Seq: []string{"g", "g"}, Categories: []string{"Navigation"}, Title: "first row"},
		{ID: bindingTicketsSelectLast, Seq: []string{"G"}, Categories: []string{"Navigation"}, Title: "last row"},
		{ID: bindingTicketsPreviewBottom, Seq: []string{"b"}, Categories: []string{"Navigation"}, Title: "preview bottom"},
	})
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.focus == focusPreview {
		return m.handlePreviewKey(msg)
	}

	if nextSearch, cmd, result := m.search.Update(msg); result.Handled {
		m.search = nextSearch
		if result.QueryChanged {
			m.recomputeSearchMatches()
		}
		if result.QueryChanged || result.CursorChanged {
			m.jumpToCurrentMatch()
		}
		return m, cmd
	}

	if msg.String() == "q" {
		return m, nav.Back()
	}

	match, consumed := m.keys.Process(msg)
	if !consumed {
		return m, nil
	}
	if match == nil {
		return m, nil // chord in progress
	}

	switch match.ID {
	case bindingTicketsDown:
		m.moveSelection(1)
	case bindingTicketsUp:
		m.moveSelection(-1)
	case bindingTicketsPageDown:
		m.moveSelection(list.DefaultScroll)
	case bindingTicketsPageUp:
		m.moveSelection(-list.DefaultScroll)
	case bindingTicketsCollapse:
		m.collapseSelectedEpic()
	case bindingTicketsExpand:
		if m.focusPreviewOrExpand() {
			return m, nil
		}
		m.expandSelectedEpic()
	case bindingTicketsToggle:
		if m.focusPreviewOrExpand() {
			return m, nil
		}
		m.expandSelectedEpic()
	case bindingTicketsRefresh:
		return m, m.cmdRefresh()
	case bindingTicketsEditInPlace:
		return m, m.cmdEditSelectedFile(terminalrun.InPlace)
	case bindingTicketsEditHSplit:
		return m, m.cmdEditSelectedFile(terminalrun.HSplit)
	case bindingTicketsEditVSplit:
		return m, m.cmdEditSelectedFile(terminalrun.VSplit)
	case bindingTicketsEditTab:
		return m, m.cmdEditSelectedFile(terminalrun.Tab)
	case bindingTicketsCancelChord:
		return m, nil
	case bindingTicketsImplement:
		return m.handleImplementKey()
	case bindingTicketsToggleCheck:
		return m.handleToggleCheck()
	case bindingTicketsToggleHideDone:
		m.toggleHideDone()
	case bindingTicketsSelectFirst:
		m.selectFirstRow()
	case bindingTicketsSelectLast:
		m.selectLastRow()
	case bindingTicketsPreviewBottom:
		m.previewVP.GotoBottom()
	}
	return m, nil
}

// selectFirstRow/selectLastRow implement "gg"/"G": jump the sidebar
// selection to the first/last visible row, leaving focus on the sidebar.
func (m *Model) selectFirstRow() {
	if len(m.visibleRows()) == 0 {
		return
	}
	m.selected = 0
	m.ensureSidebarVisible()
}

func (m *Model) selectLastRow() {
	n := len(m.visibleRows())
	if n == 0 {
		return
	}
	m.selected = n - 1
	m.ensureSidebarVisible()
}

// toggleHideDone flips the "tc" hide-complete filter and re-clamps
// selection, since hiding done tickets can shrink visibleRows() out from
// under the current cursor position.
func (m *Model) toggleHideDone() {
	m.hideDone = !m.hideDone
	if m.search.HasQuery() {
		m.recomputeSearchMatches()
	}
	m.clampSelected()
}

func (m *Model) moveSelection(delta int) {
	n := len(m.visibleRows())
	if n == 0 {
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= n {
		m.selected = n - 1
	}
	m.ensureSidebarVisible()
}

// selectedRow returns the row currently under the selection, if any.
func (m Model) selectedRow() (row, bool) {
	rows := m.visibleRows()
	if m.selected < 0 || m.selected >= len(rows) {
		return row{}, false
	}
	return rows[m.selected], true
}

// collapseSelectedEpic handles "h"/"left": on an epic row it collapses the
// epic (same as filetree); on an expanded ticket row with children (ticket
// 09) it collapses that ticket's own children one level down, mirroring the
// epic case; on any other ticket row (a leaf, or one already collapsed) it
// jumps selection up to the row's nearest containing row — a parent ticket
// row if nested, otherwise the epic row — mirroring filetree's "collapse
// jumps to parent" behavior for a leaf.
func (m *Model) collapseSelectedEpic() {
	r, ok := m.selectedRow()
	if !ok {
		return
	}
	if r.isEpic() {
		if m.isCollapsed(m.epics[r.epicIdx]) {
			return
		}
		m.setCollapsed(r.epicIdx, true)
		return
	}
	if r.hasChildren && r.expanded {
		m.setCollapsedTicket(m.epics[r.epicIdx].Tickets[r.ticketIdx].Path, true)
		return
	}
	if r.parentTicketIdx != noParentTicket {
		m.jumpToTicket(r.epicIdx, r.parentTicketIdx)
		return
	}
	m.jumpToEpic(r.epicIdx)
}

// jumpToEpic moves the selection to epicIdx's epic row within visibleRows().
func (m *Model) jumpToEpic(epicIdx int) {
	for i, r := range m.visibleRows() {
		if r.isEpic() && r.epicIdx == epicIdx {
			m.selected = i
			m.ensureSidebarVisible()
			return
		}
	}
}

// jumpToTicket moves the selection to (epicIdx, ticketIdx)'s ticket row
// within visibleRows(), collapseSelectedEpic's nested-ticket counterpart to
// jumpToEpic. The target is always visible: a child row only ever appears
// once every one of its ancestors is expanded.
func (m *Model) jumpToTicket(epicIdx, ticketIdx int) {
	for i, r := range m.visibleRows() {
		if !r.isEpic() && r.epicIdx == epicIdx && r.ticketIdx == ticketIdx {
			m.selected = i
			m.ensureSidebarVisible()
			return
		}
	}
}

func (m *Model) expandSelectedEpic() {
	r, ok := m.selectedRow()
	if !ok {
		return
	}
	if r.isEpic() {
		if !m.isCollapsed(m.epics[r.epicIdx]) {
			return
		}
		m.setCollapsed(r.epicIdx, false)
		return
	}
	if r.hasChildren && !r.expanded {
		m.setCollapsedTicket(m.epics[r.epicIdx].Tickets[r.ticketIdx].Path, false)
	}
}

// setCollapsed sets the collapse state for the epic at epicIdx and
// re-clamps the selection, since collapsing hides rows below it.
func (m *Model) setCollapsed(epicIdx int, collapsed bool) {
	path := m.epics[epicIdx].Path
	if m.collapsedEpics == nil {
		m.collapsedEpics = map[string]bool{}
	}
	// An explicit false is stored (not deleted) so defaultCollapsedEpics can
	// tell "user expanded this" apart from "never toggled" on the next
	// auto-refresh reload — both would otherwise read as "key absent".
	m.collapsedEpics[path] = collapsed
	// Collapsing/expanding reshuffles visibleRows(), which search matches
	// index into by position — recompute so they stay aligned.
	if m.search.HasQuery() {
		m.recomputeSearchMatches()
	}
	m.clampSelected()
}

// setCollapsedTicket is setCollapsed's ticket-level counterpart (ticket 09),
// keyed by Ticket.Path rather than epic index/path since it's read from
// collapsedTickets by ui/tree's entry-builder inside ticketRows.
func (m *Model) setCollapsedTicket(path string, collapsed bool) {
	if m.collapsedTickets == nil {
		m.collapsedTickets = map[string]bool{}
	}
	if collapsed {
		m.collapsedTickets[path] = true
	} else {
		delete(m.collapsedTickets, path)
	}
	if m.search.HasQuery() {
		m.recomputeSearchMatches()
	}
	m.clampSelected()
}
