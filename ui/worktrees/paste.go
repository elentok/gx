package worktrees

import (
	"fmt"
	"path/filepath"

	"gx/git"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// pasteResultMsg is sent when a paste operation completes.
type pasteResultMsg struct {
	n   int // number of files pasted
	err error
}

// handlePasteModeKey handles key events in paste mode (clipboard active, waiting for destination).
// Only navigation (j/k/arrows), paste (p), and cancel (esc) are active.
func (m Model) handlePasteModeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.PasteCancel):
		m.clipboard = nil
		m.mode = modeNormal
		return m, nil
	case key.Matches(msg, keys.PasteConfirm):
		if m.clipboard != nil {
			wt := m.selectedWorktree()
			if wt != nil {
				m.statusMsg = "Pasting…"
				m.mode = modeNormal
				return m, cmdPaste(*m.clipboard, *wt)
			}
		}
		m.mode = modeNormal
		return m, nil
	case key.Matches(msg, keys.Up), key.Matches(msg, keys.Down):
		prevCursor := m.table.Cursor()
		var tableCmd tea.Cmd
		m.table, tableCmd = m.table.Update(msg)
		if m.table.Cursor() != prevCursor && len(m.worktrees) > 0 {
			m.table.SetRows(m.buildRows())
			m.sidebarLoading = true
			m.sidebarUpstream = ""
			m.sidebarAheadCommits = nil
			m.sidebarBehindCommits = nil
			m.sidebarChanges = nil
			m.viewport.SetContent(m.sidebarContent())
			return m, tea.Batch(tableCmd, cmdLoadSidebarData(m.repo, m.worktrees[m.table.Cursor()]))
		}
		return m, tableCmd
	}
	return m, nil
}

// cmdPaste copies clipboard files from their source into the destination worktree.
func cmdPaste(cb clipboardState, dst git.Worktree) tea.Cmd {
	return func() tea.Msg {
		for _, relPath := range cb.files {
			src := filepath.Join(cb.srcPath, relPath)
			dstPath := filepath.Join(dst.Path, relPath)
			if err := copyPath(src, dstPath); err != nil {
				return pasteResultMsg{err: fmt.Errorf("copy %s: %w", relPath, err)}
			}
		}
		return pasteResultMsg{n: len(cb.files)}
	}
}
