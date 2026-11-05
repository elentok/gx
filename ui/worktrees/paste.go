package worktrees

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/help"
	keymgr "github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/worktrees/modal"

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

// pasteModal holds the yanked files while the user picks the destination
// worktree. It draws nothing itself: the page shows the PASTE chip.
type pasteModal struct {
	clipboard modal.Clipboard
}

// enterPasteMode holds cb as the clipboard and announces the paste keys once.
func (m Model) enterPasteMode(cb modal.Clipboard) (Model, tea.Cmd) {
	hints := make([]string, len(pasteModeBindings))
	for i, b := range pasteModeBindings {
		hints[i] = b.Keys() + " " + b.Title
	}
	return m.openModal(pasteModal{clipboard: cb}), notify.Info(strings.Join(hints, " · "))
}

// Update handles keys while the clipboard waits for a destination. Only
// navigation (j/k/arrows), paste (p), and cancel (esc) are active.
func (p pasteModal) Update(msg tea.Msg) (modal.Modal, tea.Cmd, modal.Result) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil, modal.Result{Unhandled: true}
	}
	switch k.String() {
	case "esc", "q":
		return p, nil, modal.Result{Close: true}
	case "p":
		return p, nil, modal.Result{Close: true, Action: modal.PasteHere{Clipboard: p.clipboard}}
	case "up", "k", "down", "j":
		return p, nil, modal.Result{Action: modal.MoveCursor{Key: k}}
	}
	return p, nil, modal.Result{}
}

func (p pasteModal) InputFocused() bool { return false }

func (p pasteModal) View(_, _ int) string { return "" }

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
