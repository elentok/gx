package log

import (
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"

	"charm.land/bubbles/v2/key"
)

var helpSectionOrder = []string{"Navigation", "Search", "Jump", "Go to", "Other"}

func buildKeySections(manager keys.Manager) []help.KeySection {
	categoryBindings := make(map[string][]key.Binding)
	seenInCategory := make(map[string]map[keys.BindingID]bool)
	for _, b := range manager.Bindings() {
		if b.Title == "" {
			continue
		}
		for _, cat := range b.Categories {
			if cat == "" {
				continue
			}
			if seenInCategory[cat] == nil {
				seenInCategory[cat] = map[keys.BindingID]bool{}
			}
			if seenInCategory[cat][b.ID] {
				continue
			}
			seenInCategory[cat][b.ID] = true
			categoryBindings[cat] = append(categoryBindings[cat], key.NewBinding(key.WithKeys(b.Seq...), key.WithHelp(b.Keys(), b.Title)))
		}
	}

	sections := make([]help.KeySection, 0, len(helpSectionOrder))
	for _, cat := range helpSectionOrder {
		if bindings, ok := categoryBindings[cat]; ok {
			sections = append(sections, help.NewKeySection(cat, bindings...))
			continue
		}
		if cat == "Search" {
			sections = append(sections, help.NewKeySection(cat, logKeySearch, logKeyResultNext, logKeyResultPrev))
		}
	}
	return sections
}

func (m Model) ChordHints(_ string) []key.Binding {
	hints := m.keys.ChordHints()
	result := make([]key.Binding, len(hints))
	for i, h := range hints {
		result[i] = key.NewBinding(key.WithHelp(h.Key, h.Desc))
	}
	return result
}
