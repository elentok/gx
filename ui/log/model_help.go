package log

import (
	"github.com/elentok/gx/ui/help"

	"charm.land/bubbles/v2/key"
)

const (
	MIN_WIDTH  = 56
	MAX_WIDTH  = 104
	MIN_HEIGHT = 8
)

var keySections = []help.KeySection{
	help.NewKeySection("Navigation", logKeyUp, logKeyDown, logKeyTop, logKeyBottom, logKeyOpen),
	help.NewKeySection("Search", logKeySearch, logKeyResultNext, logKeyResultPrev),
	help.NewKeySection("Jump", logKeyHead, logKeyNextTag, logKeyPrevTag),
	help.NewKeySection("Go to", logKeyWorktrees, logKeyGotoLog, logKeyStatus),
	help.NewKeySection("Other", logKeyReload, logKeyBack, logKeyHelp),
}

// ChordHints returns the available chord completions for the given prefix.
// Implements ui.ChordHinter.
func (m Model) ChordHints(prefix string) []key.Binding {
	switch prefix {
	case "g":
		return []key.Binding{
			key.NewBinding(key.WithHelp("g", "top")),
			key.NewBinding(key.WithHelp("h", "goto HEAD")),
			key.NewBinding(key.WithHelp("w", "goto worktrees")),
			key.NewBinding(key.WithHelp("l", "goto log")),
			key.NewBinding(key.WithHelp("s", "goto status")),
		}
	case "]":
		return []key.Binding{key.NewBinding(key.WithHelp("t", "next tag"))}
	case "[":
		return []key.Binding{key.NewBinding(key.WithHelp("t", "prev tag"))}
	}
	return nil
}
