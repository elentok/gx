package worktrees

import (
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/worktrees/modal"

	tea "charm.land/bubbletea/v2"
)

// pasteResultMsg is sent when a paste operation completes.
type pasteResultMsg struct {
	n   int // number of files pasted
	err error
}

// enterPasteMode holds cb as the clipboard and announces the paste keys once.
func (m Model) enterPasteMode(cb modal.Clipboard) (Model, tea.Cmd) {
	return m.openModal(modal.NewPaste(cb)), notify.Info(modal.PasteHint())
}

// cmdPaste copies clipboard files from their source into the destination worktree.
func cmdPaste(cb modal.Clipboard, dst git.Worktree) tea.Cmd {
	return func() tea.Msg {
		for _, relPath := range cb.Files {
			src := filepath.Join(cb.SrcPath, relPath)
			dstPath := filepath.Join(dst.Path, relPath)
			if err := copyPath(src, dstPath); err != nil {
				return pasteResultMsg{err: fmt.Errorf("copy %s: %w", relPath, err)}
			}
		}
		return pasteResultMsg{n: len(cb.Files)}
	}
}
