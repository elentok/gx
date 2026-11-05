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

// pasteModal holds the yanked files while the user picks the destination
// worktree. It draws nothing itself: the page shows the PASTE chip.
type pasteModal struct {
	clipboard clipboardState
}

// enterPasteMode holds cb as the clipboard and announces the paste keys once.
func (m Model) enterPasteMode(cb clipboardState) (Model, tea.Cmd) {
	hints := make([]string, len(pasteModeBindings))
	for i, b := range pasteModeBindings {
		hints[i] = b.Keys() + " " + b.Title
	}
	return m.openModal(pasteModal{clipboard: cb}), notify.Info(strings.Join(hints, " · "))
}

// Update handles keys while the clipboard waits for a destination. Only
// navigation (j/k/arrows), paste (p), and cancel (esc) are active.
func (p pasteModal) Update(msg tea.Msg) (modal, tea.Cmd, modalResult) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil, modalResult{unhandled: true}
	}
	switch k.String() {
	case "esc", "q":
		return p, nil, modalResult{close: true}
	case "p":
		return p, nil, modalResult{close: true, action: pasteHere{clipboard: p.clipboard}}
	case "up", "k", "down", "j":
		return p, nil, modalResult{action: moveCursor{key: k}}
	}
	return p, nil, modalResult{}
}

func (p pasteModal) InputFocused() bool { return false }

func (p pasteModal) View(_, _ int) string { return "" }

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
