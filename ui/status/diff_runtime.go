package status

import "github.com/elentok/gx/ui/diffview"

func (m *Model) switchDiffSection() {
	m.diff.ToggleSection()
	m.syncDiffViewports()
	m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
}

func (m Model) editorLineForCurrentSelection() int {
	if m.focus != focusDiff {
		return 0
	}
	diffviewModel := m.diff.ActiveSectionModel()
	diff := diffviewModel.DataRef()
	if m.diff.NavMode() == diffview.NavModeLine {
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
