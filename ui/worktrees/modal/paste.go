package modal

import (
	"strings"

	"github.com/elentok/gx/ui/help"
	keymgr "github.com/elentok/gx/ui/keys"

	tea "charm.land/bubbletea/v2"
)

// pasteModeBindings document paste mode's keys for help and the entry
// notification; Update matches keys directly because paste mode swallows every
// other binding.
var pasteModeBindings = []keymgr.Binding{
	{ID: "paste", Seq: []string{"p"}, Title: "paste"},
	{ID: "paste-cancel", Seq: []string{"esc"}, Title: "cancel", Display: "esc/q"},
}

// PasteModeHelpSection is paste mode's section of the help screen.
func PasteModeHelpSection() help.KeySection {
	return help.NewKeySection("Paste mode", pasteModeBindings...)
}

// PasteHint is the one-line key hint shown when paste mode opens.
func PasteHint() string {
	hints := make([]string, len(pasteModeBindings))
	for i, b := range pasteModeBindings {
		hints[i] = b.Keys() + " " + b.Title
	}
	return strings.Join(hints, " · ")
}

// Paste holds the yanked files while the user picks the destination
// worktree. It draws nothing itself: the page shows the PASTE chip.
type Paste struct {
	clipboard Clipboard
}

// NewPaste holds cb as the clipboard.
func NewPaste(cb Clipboard) Paste {
	return Paste{clipboard: cb}
}

// Clipboard is the held clipboard.
func (p Paste) Clipboard() Clipboard { return p.clipboard }

// Update handles keys while the clipboard waits for a destination. Only
// navigation (j/k/arrows), paste (p), and cancel (esc) are active.
func (p Paste) Update(msg tea.Msg) (Modal, tea.Cmd, Result) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil, Result{Unhandled: true}
	}
	switch k.String() {
	case "esc", "q":
		return p, nil, Result{Close: true}
	case "p":
		return p, nil, Result{Close: true, Action: PasteHere{Clipboard: p.clipboard}}
	case "up", "k", "down", "j":
		return p, nil, Result{Action: MoveCursor{Key: k}}
	}
	return p, nil, Result{}
}

func (p Paste) InputFocused() bool { return false }

func (p Paste) View(_, _ int) string { return "" }
