package commit

import (
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
)

var commitHelpSectionOrder = []string{"Global", "Go to", "Header", "Diff", "Yank", "Navigation", "Actions"}

func buildCommitKeySections(manager keys.Manager) []help.KeySection {
	sections := []help.KeySection{}
	byCategory := map[string][]keys.Binding{}
	seen := map[string]map[keys.BindingID]bool{}
	for _, b := range manager.Bindings() {
		if b.Title == "" {
			continue
		}
		for _, cat := range b.Categories {
			if cat == "" {
				continue
			}
			if seen[cat] == nil {
				seen[cat] = map[keys.BindingID]bool{}
			}
			if seen[cat][b.ID] {
				continue
			}
			seen[cat][b.ID] = true
			byCategory[cat] = append(byCategory[cat], keys.Binding{ID: b.ID, Seq: b.Seq, Title: b.Title, Display: b.Display})
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
