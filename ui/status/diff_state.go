package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/status/diffarea"
	"github.com/elentok/gx/ui/status/stagemodal"
)

func (m *Model) openDiscardDiffConfirm() tea.Cmd {
	if m.diffarea.ActiveSection != diffarea.SectionUnstaged {
		return nil
	}
	file, ok := m.selectedStatusFile()
	if !ok {
		return nil
	}
	diffviewModel := m.diffarea.ActiveSectionModel()

	var (
		title       string
		lines       []string
		patch       string
		unidiffZero bool
		err         error
	)

	if m.diffarea.NavMode() == diffview.NavModeHunk {
		if diffviewModel.DataRef().ActiveHunk < 0 || diffviewModel.DataRef().ActiveHunk >= len(diffviewModel.DataRef().Parsed.Hunks) {
			return nil
		}
		patch, err = diffcore.BuildHunkPatch(diffviewModel.DataRef().Parsed, diffviewModel.DataRef().ActiveHunk)
		title = "Discard selected hunk?"
		lines = []string{"This will discard the selected hunk from your working tree."}
	} else {
		if diffviewModel.DataRef().ActiveLine < 0 || diffviewModel.DataRef().ActiveLine >= len(diffviewModel.DataRef().Parsed.Changed) {
			return nil
		}
		startLine, endLine := diffviewModel.DataRef().ActiveLine, diffviewModel.DataRef().ActiveLine
		if diffviewModel.DataRef().VisualActive {
			startLine, endLine = diffviewModel.Data().VisualLineBounds()
		}
		if diffviewModel.DataRef().VisualActive && endLine > startLine {
			patch, err = diffcore.BuildLineRangePatch(diffviewModel.DataRef().Parsed, startLine, endLine)
			title = "Discard selected lines?"
			lines = []string{"This will discard the selected lines from your working tree."}
		} else {
			patch, err = diffcore.BuildSingleLinePatch(diffviewModel.DataRef().Parsed, diffviewModel.DataRef().ActiveLine)
			title = "Discard selected line?"
			lines = []string{"This will discard the selected line from your working tree."}
		}
		unidiffZero = true
	}

	if err != nil {
		return notify.Error(err.Error())
	}

	openConfirmRequest(m, stagemodal.ConfirmRequest{
		Title:            title,
		Lines:            lines,
		Action:           stagemodal.ConfirmDiscardUnstaged,
		Paths:            []string{file.Path},
		Patch:            patch,
		PatchUnidiffZero: unidiffZero,
	})
	return nil
}
