package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

const (
	bindingTicketsHelp               keys.BindingID = "help"
	bindingTicketsBack               keys.BindingID = "back"
	bindingTicketsResume             keys.BindingID = "resume"
	bindingTicketsRefresh            keys.BindingID = "refresh"
	bindingTicketsEditInPlace        keys.BindingID = "edit"
	bindingTicketsEditHSplit         keys.BindingID = "edit-hsplit"
	bindingTicketsEditVSplit         keys.BindingID = "edit-vsplit"
	bindingTicketsEditTab            keys.BindingID = "edit-tab"
	bindingTicketsCancelChord        keys.BindingID = "cancel-chord"
	bindingTicketsFinishAndPause     keys.BindingID = "finish-and-pause-queue"
	bindingTicketsToggleHideDone     keys.BindingID = "toggle-hide-done"
	bindingTicketsToggleProjectScope keys.BindingID = "toggle-project-scope"
	bindingTicketsTogglePreview      keys.BindingID = "toggle-preview"
	bindingTicketsSelectFirst        keys.BindingID = "select-first"
	bindingTicketsSelectLast         keys.BindingID = "select-last"
	bindingTicketsPreviewBottom      keys.BindingID = "preview-bottom"
	bindingTicketsSuggestedActions   keys.BindingID = "suggested-actions"
	bindingTicketsYankSummary        keys.BindingID = "yank-summary"
	bindingTicketsYankFilePath       keys.BindingID = "yank-file-path"
)

// archivedBlockedBindings lists every mutating binding archivedReadOnlyGuard
// must block on an archived row. A future mutating binding needs an entry
// here to be guarded — nothing else enforces that. The "m" menu is not here:
// it opens on an archived row with its writes dim.
var archivedBlockedBindings = map[keys.BindingID]bool{
	bindingTicketsFinishAndPause: true,
}

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
		{ID: bindingTicketsFinishAndPause, Seq: []string{"D"}, Categories: []string{"Navigation"}, Title: "finish and pause queue"},
		{ID: bindingTicketsToggleHideDone, Seq: []string{"t", "c"}, Categories: []string{"Navigation"}, Title: "hide completed"},
		{ID: bindingTicketsToggleProjectScope, Seq: []string{"t", "p"}, Categories: []string{"Navigation"}, Title: "cwd project / all projects"},
		{ID: bindingTicketsTogglePreview, Seq: []string{"t", "v"}, Categories: []string{"Navigation"}, Title: "toggle preview"},
		{ID: bindingTicketsSelectFirst, Seq: []string{"g", "g"}, Categories: []string{"Navigation"}, Title: "first row"},
		{ID: bindingTicketsSelectLast, Seq: []string{"G"}, Categories: []string{"Navigation"}, Title: "last row"},
		{ID: bindingTicketsPreviewBottom, Seq: []string{"b"}, Categories: []string{"Navigation"}, Title: "preview bottom"},
		{ID: bindingTicketsSuggestedActions, Seq: []string{"m"}, Categories: []string{"Navigation"}, Title: "actions"},
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
	if m.preview.Focus == preview.FocusPreview {
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
		if cmd, blocked := m.server.keyGuard(match.ID); blocked {
			return m, cmd
		}
		if archivedBlockedBindings[match.ID] {
			if cmd, blocked := m.archivedReadOnlyGuard(); blocked {
				return m, cmd
			}
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
		case bindingTicketsFinishAndPause:
			return m.handleServerFinishAndPauseKey()
		case bindingTicketsToggleHideDone:
			// Hiding done tickets can shrink the tree out from under the cursor.
			m.sidebar.HideDone = !m.sidebar.HideDone
			if m.search.HasQuery() {
				m.recomputeSearchMatches()
			}
			m.clampSelected()
		case bindingTicketsToggleProjectScope:
			return m.toggleProjectScope()
		case bindingTicketsTogglePreview:
			m.preview.Toggle()
			m.sidebar.Tree.SetVisibleHeight(m.layout().sidebarViewportHeight())
		case bindingTicketsSelectFirst:
			m.sidebar.SelectFirstRow()
		case bindingTicketsSelectLast:
			m.sidebar.SelectLastRow()
		case bindingTicketsPreviewBottom:
			m.preview.VP.GotoBottom()
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
	// selectedID is captured before tree.Model.Update runs a collapse/expand/
	// toggle binding directly against its own internal collapsed map (see
	// ui/tree/ops.go) — the only way ui/tickets learns which single entry
	// just changed, short of duplicating that binding logic here.
	if msg.String() == "enter" {
		if cmd, handled := rowactions.ResumeEnter(modelMenuTab{m}); handled {
			return m, cmd
		}
	}
	selectedID := ""
	if entries := m.sidebar.Tree.Entries(); len(entries) > 0 {
		if idx := m.sidebar.Tree.SelectedIndex(); idx >= 0 && idx < len(entries) {
			selectedID = entries[idx].ID
		}
	}
	next, cmd, result := m.sidebar.Tree.Update(msg)
	if result.ExpandNoop {
		m.preview.FocusPreviewPane()
		return m, cmd
	}
	m.sidebar.Tree = next
	if result.RebuildRequested {
		cmd = m.afterSidebarToggle(selectedID, cmd)
	}
	if result.OpenSelected {
		m.preview.FocusPreviewPane()
	}
	// BindingMoveDown/BindingMoveUp already self-skip non-selectable rows
	// inside tree.Model.Update (see SetIsSelectable/SkipUnselectable) — no
	// extra nudge needed here for j/k/up/down.
	return m, cmd
}

// archivedReadOnlyGuard is the mutating keybindings' shared no-op check:
// tickets/epics sourced from the (ticket 04) Archived section live outside
// the active tracker as a plain filesystem move under .scratch/.archive/, so
// status changes, queue actions, toggle-check, and suggested-actions must not
// write to or navigate away from them. blocked reports whether the caller
// should stop and return cmd unchanged instead of running its handler.
func (m Model) archivedReadOnlyGuard() (cmd tea.Cmd, blocked bool) {
	r, ok := m.sidebar.SelectedRow()
	if !ok || !r.Archived {
		return nil, false
	}
	return notify.Info("read-only: this ticket is archived"), true
}
