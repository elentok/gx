package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/terminalrun"
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
// m.sidebarTree's own nav bindings via m.sidebarTree.Update (which now
// handles paging directly, see ui/tree/model.go's BindingPageDown/
// BindingPageUp cases — ticket 31). ui/tree's own doc comment ("see
// ui/status/filetree_keys.go") suggests calling m.sidebarTree.Update first
// and falling back to m.sidebarTree.Keys().Process on a miss — that pattern
// is safe only for single-key bindings; for multi-key chords the second
// Process call sees a reset prefix and can wrongly re-arm a pending chord
// instead of surfacing the completed match. Trying m.keys first side-steps
// that entirely since neither manager's Process is ever called twice for the
// same keystroke.
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

	// Expand-on-already-expanded: tree.Model's own Update reports ExpandNoop
	// on a row that's HasChildren && already Expanded (nothing left to
	// expand) — the pre-migration behavior was for a second press there to
	// hand focus to the preview instead, so that mutation is discarded
	// (next is dropped rather than assigned back) and focus redirected
	// instead. A leaf row never sets ExpandNoop; it falls through to the
	// tree's own OpenSelected below, same as always.
	next, cmd, result := m.sidebarTree.Update(msg)
	if result.ExpandNoop {
		m.focus = focusPreview
		return m, cmd
	}
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
	// BindingMoveDown/BindingMoveUp already self-skip non-selectable rows
	// inside tree.Model.Update (see SetIsSelectable/SkipUnselectable) — no
	// extra nudge needed here for j/k/up/down.
	return m, cmd
}

// selectFirstRow/selectLastRow implement "gg"/"G": jump the sidebar
// selection to the first/last row, leaving focus on the sidebar.
func (m *Model) selectFirstRow() {
	if len(m.sidebarTree.Entries()) == 0 {
		return
	}
	m.sidebarTree.SetSelectedIndex(0)
	m.sidebarTree.SkipUnselectable(1)
}

func (m *Model) selectLastRow() {
	n := len(m.sidebarTree.Entries())
	if n == 0 {
		return
	}
	m.sidebarTree.SetSelectedIndex(n - 1)
	m.sidebarTree.SkipUnselectable(-1)
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
