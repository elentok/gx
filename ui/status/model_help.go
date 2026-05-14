package status

import (
	"fmt"

	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
)

var helpSectionOrder = []string{"Global", "Go to", "Git", "Yank", "Search", "Filetree", "Diff"}

// buildKeySections generates help sections from the provided key managers.
// Bindings with an empty Title are skipped. Within each category, only the
// first binding per BindingID is shown (aliases are suppressed).
func buildKeySections(managers ...keys.Manager) []help.KeySection {
	categoryBindings := make(map[string][]keys.Binding)
	seenInCategory := make(map[string]map[keys.BindingID]bool)

	for _, mgr := range managers {
		for _, b := range mgr.Bindings() {
			if b.Title == "" {
				continue
			}
			for _, cat := range b.Categories {
				if cat == "" {
					continue
				}
				if seenInCategory[cat] == nil {
					seenInCategory[cat] = make(map[keys.BindingID]bool)
				}
				if seenInCategory[cat][b.ID] {
					continue
				}
				seenInCategory[cat][b.ID] = true
				categoryBindings[cat] = append(categoryBindings[cat],
					keys.Binding{ID: b.ID, Seq: b.Seq, Title: b.Title, Display: b.Display})
			}
		}
	}

	sections := make([]help.KeySection, 0, len(helpSectionOrder))
	for _, cat := range helpSectionOrder {
		if bindings, ok := categoryBindings[cat]; ok {
			sections = append(sections, help.NewKeySection(cat, bindings...))
		}
	}
	return sections
}

func (m Model) helpSectionLabel() string {
	if m.focus == focusFiletree {
		return "filetree"
	}
	return fmt.Sprintf("diff:%s:%s", m.navModeLabel(), m.renderModeLabel())
}

func (m Model) navModeLabel() string {
	if m.diffarea.NavMode() == diffview.NavModeLine {
		return "line"
	}
	return "hunk"
}
