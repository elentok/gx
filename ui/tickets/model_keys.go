package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tree"
)

const (
	bindingTicketsHelp             keys.BindingID = "help"
	bindingTicketsBack             keys.BindingID = "back"
	bindingTicketsResume           keys.BindingID = "resume"
	bindingTicketsRefresh          keys.BindingID = "refresh"
	bindingTicketsEditInPlace      keys.BindingID = "edit"
	bindingTicketsEditHSplit       keys.BindingID = "edit-hsplit"
	bindingTicketsEditVSplit       keys.BindingID = "edit-vsplit"
	bindingTicketsEditTab          keys.BindingID = "edit-tab"
	bindingTicketsCancelChord      keys.BindingID = "cancel-chord"
	bindingTicketsReplaceQueue     keys.BindingID = "replace-queue"
	bindingTicketsAddToQueue       keys.BindingID = "add-to-queue"
	bindingTicketsToggleCheck      keys.BindingID = "toggle-check"
	bindingTicketsToggleHideDone   keys.BindingID = "toggle-hide-done"
	bindingTicketsSelectFirst      keys.BindingID = "select-first"
	bindingTicketsSelectLast       keys.BindingID = "select-last"
	bindingTicketsPreviewBottom    keys.BindingID = "preview-bottom"
	bindingTicketsChangeStatus     keys.BindingID = "change-status"
	bindingTicketsSuggestedActions keys.BindingID = "suggested-actions"
	bindingTicketsYankSummary      keys.BindingID = "yank-summary"
	bindingTicketsYankFilePath     keys.BindingID = "yank-file-path"
)

// newTicketsManager builds the key manager for the sidebar's "extra"
// bindings — everything beyond ui/tree's own base nav (j/k/h/l/enter/ctrl+d/
// ctrl+u, see ui/tree/model_keys.go), which m.sidebarTree's own manager
// handles instead (see handleKey's two-manager split below).
func newTicketsManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingTicketsHelp, Seq: []string{"?"}, Categories: []string{"Other"}, Title: "help"},
		{ID: bindingTicketsRefresh, Seq: []string{"R"}, Categories: []string{"Navigation"}, Title: "refresh"},
		// e-prefix chords: edit the selected row's underlying file, reusing
		// the same launch-mode plumbing every other tab's edit-chord uses.
		{ID: bindingTicketsEditInPlace, Seq: []string{"e", "e"}, Categories: []string{"Navigation"}, Title: "edit file"},
		{ID: bindingTicketsEditHSplit, Seq: []string{"e", "s"}, Categories: []string{"Navigation"}, Title: "edit file (split)"},
		{ID: bindingTicketsEditVSplit, Seq: []string{"e", "v"}, Categories: []string{"Navigation"}, Title: "edit file (vsplit)"},
		{ID: bindingTicketsEditTab, Seq: []string{"e", "t"}, Categories: []string{"Navigation"}, Title: "edit file (tab)"},
		{ID: bindingTicketsCancelChord, Seq: []string{"e", "esc"}, Categories: []string{}, Title: ""},
		{ID: bindingTicketsReplaceQueue, Seq: []string{"r"}, Categories: []string{"Navigation"}, Title: "replace queue"},
		{ID: bindingTicketsAddToQueue, Seq: []string{"a"}, Categories: []string{"Navigation"}, Title: "add to queue"},
		{ID: bindingTicketsToggleCheck, Seq: []string{"space"}, Categories: []string{"Navigation"}, Title: "check/uncheck"},
		{ID: bindingTicketsToggleHideDone, Seq: []string{"t", "c"}, Categories: []string{"Navigation"}, Title: "hide completed"},
		{ID: bindingTicketsSelectFirst, Seq: []string{"g", "g"}, Categories: []string{"Navigation"}, Title: "first row"},
		{ID: bindingTicketsSelectLast, Seq: []string{"G"}, Categories: []string{"Navigation"}, Title: "last row"},
		{ID: bindingTicketsPreviewBottom, Seq: []string{"b"}, Categories: []string{"Navigation"}, Title: "preview bottom"},
		{ID: bindingTicketsChangeStatus, Seq: []string{"s"}, Categories: []string{"Navigation"}, Title: "change status"},
		{ID: bindingTicketsSuggestedActions, Seq: []string{"m"}, Categories: []string{"Navigation"}, Title: "suggested actions"},
		// y-prefix chords
		{ID: bindingTicketsYankSummary, Seq: []string{"y", "y"}, Categories: []string{"Yank"}, Title: "yank epic - ticket"},
		{ID: bindingTicketsYankFilePath, Seq: []string{"y", "f"}, Categories: []string{"Yank"}, Title: "yank file path"},
		{ID: bindingTicketsCancelChord, Seq: []string{"y", "esc"}, Categories: []string{}, Title: ""},
	})
}

// handleKey processes sidebar-focused key input via two separate
// keys.Manager instances, tried in a fixed order, never re-Process-ing the
// same manager for the same keystroke: m.keys' "extra" bindings first, then
// m.sidebarTree's own nav bindings. ui/tree's own doc comment ("see
// ui/status/filetree_keys.go") suggests calling m.sidebarTree.Update first
// and falling back to m.sidebarTree.Keys().Process on a miss — that pattern
// is safe only for single-key bindings; for multi-key chords the second
// Process call sees a reset prefix and can wrongly re-arm a pending chord
// instead of surfacing the completed match. Trying m.keys first side-steps
// that entirely since neither manager's Process is ever called twice for the
// same keystroke (the ctrl+d/ctrl+u fallback below is the one exception,
// safe because those are single-key bindings on m.sidebarTree's own
// manager).
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

	if match, consumed := m.keys.Process(msg); consumed {
		if match == nil {
			return m, nil // chord in progress
		}
		switch match.ID {
		case bindingTicketsHelp:
			m.keys.Reset()
			m.help.Open(m.width, m.height)
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
		case bindingTicketsReplaceQueue:
			return m.handleReplaceQueueKey()
		case bindingTicketsAddToQueue:
			return m.handleAddToQueueKey()
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
		case bindingTicketsChangeStatus:
			return m.handleChangeStatusKey()
		case bindingTicketsSuggestedActions:
			return m.handleSuggestedActionsKey()
		case bindingTicketsYankSummary:
			return m, m.yankTicketSummary()
		case bindingTicketsYankFilePath:
			return m, m.yankTicketFilePath()
		}
		return m, nil
	}

	// Expand-on-already-expanded: tree.Model's own Update no-ops "l"/"right"/
	// "enter" on a row that's HasChildren && already Expanded (nothing left
	// to expand) — the pre-migration behavior was for a second press there to
	// hand focus to the preview instead, so that's special-cased here, ahead
	// of m.sidebarTree.Update, rather than inside it.
	if s := msg.String(); s == "l" || s == "right" || s == "enter" {
		entries := m.sidebarTree.Entries()
		idx := m.sidebarTree.SelectedIndex()
		if idx >= 0 && idx < len(entries) && entries[idx].HasChildren && entries[idx].Expanded {
			m.focus = focusPreview
			return m, nil
		}
	}

	next, cmd, result := m.sidebarTree.Update(msg)
	m.sidebarTree = next
	if result.RebuildRequested {
		m.clampSelected()
		if m.search.HasQuery() {
			m.recomputeSearchMatches()
		}
	}
	if result.OpenSelected {
		m.focus = focusPreview
	}
	if result.SelectionChanged {
		dir := 1
		if s := msg.String(); s == "k" || s == "up" {
			dir = -1
		}
		m.skipUnselectableRow(dir)
	}
	if !result.Handled {
		if match, consumed := m.sidebarTree.Keys().Process(msg); consumed && match != nil {
			switch match.ID {
			case tree.BindingPageDown:
				m.sidebarTree.ScrollPage(list.DefaultScroll)
				m.skipUnselectableRow(1)
			case tree.BindingPageUp:
				m.sidebarTree.ScrollPage(-list.DefaultScroll)
				m.skipUnselectableRow(-1)
			}
		}
	}
	return m, cmd
}

// skipUnselectableRow nudges the sidebar selection off an unselectable entry
// (the blank separator row, or an empty-section placeholder) in dir's
// direction (+1 down, -1 up) — those rows are real tree.Entry rows (needed so
// BuildEntriesFromValues can size the section's child count) but must never
// hold the cursor. If dir's direction runs off the end of the entries while
// still on an unselectable row (e.g. paging up lands back on entry 0 with
// nowhere further up to go), it retries the opposite direction — an
// unselectable row must never end up selected, even at a list boundary.
func (m *Model) skipUnselectableRow(dir int) {
	idx := skipUnselectableRowDir(m.sidebarTree.Entries(), m.sidebarTree.SelectedIndex(), dir)
	if entries := m.sidebarTree.Entries(); idx >= 0 && idx < len(entries) && isUnselectableSidebarRow(entries[idx].Value.kind) {
		idx = skipUnselectableRowDir(entries, idx, -dir)
	}
	if idx != m.sidebarTree.SelectedIndex() {
		m.sidebarTree.SetSelectedIndex(idx)
	}
}

func skipUnselectableRowDir(entries []tree.Entry[sidebarNode], idx, dir int) int {
	for idx >= 0 && idx < len(entries) && isUnselectableSidebarRow(entries[idx].Value.kind) {
		next := idx + dir
		if next < 0 || next >= len(entries) {
			break
		}
		idx = next
	}
	return idx
}

// isUnselectableSidebarRow reports whether kind is a sidebar row that must
// never end up as the cursor's selection: the blank separator between the
// two sections, and an empty-section placeholder. Section headers (nodeSection)
// are real, cursor-reachable rows — collapsing/expanding a section is a
// selectable action like any epic or ticket row.
func isUnselectableSidebarRow(kind sidebarNodeKind) bool {
	return kind == nodeBlank || kind == nodeEmpty
}

// selectFirstRow/selectLastRow implement "gg"/"G": jump the sidebar
// selection to the first/last row, leaving focus on the sidebar.
func (m *Model) selectFirstRow() {
	if len(m.sidebarTree.Entries()) == 0 {
		return
	}
	m.sidebarTree.SetSelectedIndex(0)
	m.skipUnselectableRow(1)
}

func (m *Model) selectLastRow() {
	n := len(m.sidebarTree.Entries())
	if n == 0 {
		return
	}
	m.sidebarTree.SetSelectedIndex(n - 1)
	m.skipUnselectableRow(-1)
}

// toggleHideDone flips the "tc" hide-complete filter and rebuilds the
// sidebar tree, since hiding done tickets can shrink it out from under the
// current cursor position.
func (m *Model) toggleHideDone() {
	m.hideDone = !m.hideDone
	if m.search.HasQuery() {
		m.recomputeSearchMatches()
	}
	m.clampSelected()
}

// selectedRow returns the row currently under the selection, if any. A
// nodeSection selection reports false (rowFromEntry) — section rows are
// cursor-reachable but have no row representation (not checkable, no
// epic/ticket to preview). A Model built without going through clampSelected
// (e.g. a test constructing Model{epics: ...} directly, never routing it
// through a WindowSizeMsg/epicsLoadedMsg) has an empty sidebarTree — that's
// not "nothing selected", it's "entries were never built", so it falls back
// to the first epic rather than reporting no selection at all.
func (m Model) selectedRow() (row, bool) {
	entries := m.sidebarTree.Entries()
	if len(entries) == 0 {
		if len(m.epics) == 0 {
			return row{}, false
		}
		return row{epicIdx: 0, ticketIdx: -1}, true
	}
	idx := m.sidebarTree.SelectedIndex()
	if idx < 0 || idx >= len(entries) {
		return row{}, false
	}
	return rowFromEntry(entries[idx])
}
