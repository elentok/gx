package worktrees

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/help"
	keymgr "github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

// pasteResultMsg is sent when a paste operation completes.
type pasteResultMsg struct {
	n   int // number of files pasted
	err error
}

// pasteModeBindings document paste mode's keys for help and the entry
// notification; handlePasteModeKey matches keys directly because paste mode
// swallows every other binding.
var pasteModeBindings = []keymgr.Binding{
	{ID: "paste", Seq: []string{"p"}, Title: "paste"},
	{ID: "paste-cancel", Seq: []string{"esc"}, Title: "cancel", Display: "esc/q"},
}

func pasteModeHelpSection() help.KeySection {
	return help.NewKeySection("Paste mode", pasteModeBindings...)
}

// enterPasteMode holds cb as the clipboard and announces the paste keys once.
func (m Model) enterPasteMode(cb clipboardState) (Model, tea.Cmd) {
	m.clipboard = &cb
	m.mode = modePaste
	hints := make([]string, len(pasteModeBindings))
	for i, b := range pasteModeBindings {
		hints[i] = b.Keys() + " " + b.Title
	}
	return m, notify.Info(strings.Join(hints, " · "))
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
