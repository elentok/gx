package status

import "github.com/elentok/gx/ui/diffview"

func (m *Model) switchDiffSection() {
	m.diffarea.ToggleSection()
	m.syncDiffViewports()
	m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
}

func (m Model) editorLineForCurrentSelection() int {
	if m.focus != focusDiff {
		return 0
	}
	diffviewModel := m.diffarea.ActiveSectionModel()
	diff := diffviewModel.DataRef()
	if m.diffarea.NavMode() == diffview.NavModeLine {
		if diff.ActiveLine < 0 || diff.ActiveLine >= len(diff.Parsed.Changed) {
			return 0
		}
		cl := diff.Parsed.Changed[diff.ActiveLine]
		if cl.NewLine > 0 {
			return cl.NewLine
		}
		return cl.OldLine
	}
	if diff.ActiveHunk < 0 || diff.ActiveHunk >= len(diff.Parsed.Hunks) {
		return 0
	}
	h := diff.Parsed.Hunks[diff.ActiveHunk]
	if h.NewStart > 0 {
		return h.NewStart
	}
	return h.OldStart
}
