package worktrees

import "charm.land/bubbles/v2/key"

// ChordHints returns chord completion hints for the active manager prefix.
// Implements ui.ChordHinter.
func (m Model) ChordHints(_ string) []key.Binding {
	hints := m.keyManager.ChordHints()
	out := make([]key.Binding, len(hints))
	for i, h := range hints {
		out[i] = key.NewBinding(key.WithHelp(h.Key, h.Desc))
	}
	return out
}
