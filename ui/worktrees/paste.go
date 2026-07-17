package worktrees

import (
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/git"

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
	switch msg.String() {
	case "esc", "q":
		m.clipboard = nil
		m.mode = modeNormal
		return m, nil
	case "p":
		if m.clipboard != nil {
			wt := m.cursorWorktree()
			if wt != nil {
				m.mode = modeNormal
				return m, cmdPaste(*m.clipboard, *wt)
			}
		}
		m.mode = modeNormal
		return m, nil
	case "up", "k", "down", "j":
		prevCursor := m.table.Cursor()
		var tableCmd tea.Cmd
		m.table, tableCmd = m.table.Update(msg)
		if m.table.Cursor() != prevCursor && len(m.worktrees) > 0 {
			m.table.SetRows(m.buildRows())
			m.previewLoading = true
			m.previewUpstream = ""
			m.previewAheadCommits = nil
			m.previewBehindCommits = nil
			m.previewChanges = nil
			m.viewport.SetContent(m.previewContent())
			return m, tea.Batch(tableCmd, cmdLoadPreviewData(m.repo, m.worktrees[m.table.Cursor()]))
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
