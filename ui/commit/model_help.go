package commit

import (
	"charm.land/bubbles/v2/key"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keybindings"
)

var commitHelpSectionOrder = []string{"Global", "Go to", "Header", "Diff", "Yank", "Navigation"}

func buildCommitKeySections(manager keybindings.Manager) []help.KeySection {
	sections := []help.KeySection{}
	byCategory := map[string][]key.Binding{}
	seen := map[string]map[keybindings.BindingID]bool{}
	for _, b := range manager.Bindings() {
		if b.Title == "" {
			continue
		}
		for _, cat := range b.Categories {
			if cat == "" {
				continue
			}
			if seen[cat] == nil {
				seen[cat] = map[keybindings.BindingID]bool{}
			}
			if seen[cat][b.ID] {
				continue
			}
			seen[cat][b.ID] = true
			byCategory[cat] = append(byCategory[cat], key.NewBinding(key.WithKeys(b.Seq...), key.WithHelp(b.Keys(), b.Title)))
		}
	}
	for _, cat := range commitHelpSectionOrder {
		bindings := byCategory[cat]
		if len(bindings) == 0 {
			continue
		}
		sections = append(sections, help.NewKeySection(cat, bindings...))
	}
	return sections
}

func (m Model) ChordHints(_ string) []key.Binding {
	hints := m.keys.ChordHints()
	out := make([]key.Binding, len(hints))
	for i, h := range hints {
		out[i] = key.NewBinding(key.WithHelp(h.Key, h.Desc))
	}
	return out
}
