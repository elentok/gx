package status

import (
	"charm.land/bubbles/v2/viewport"

	"github.com/elentok/gx/ui/explorer"
)

func toExplorerSectionData(sec sectionState) explorer.SectionData {
	return explorer.SectionData{
		RawLines:         sec.rawLines,
		BaseLines:        sec.baseLines,
		BaseLineKinds:    sec.baseLineKinds,
		BaseDisplayToRaw: sec.baseDisplayToRaw,
		ViewLines:        sec.viewLines,
		ViewLineKinds:    sec.viewLineKinds,
		DisplayToRaw:     sec.displayToRaw,
		RawToDisplay:     sec.rawToDisplay,
		HunkDisplayRange: sec.hunkDisplayRange,
		ChangedDisplay:   sec.changedDisplay,
		Parsed:           sec.parsed,
		ActiveHunk:       sec.activeHunk,
		ActiveLine:       sec.activeLine,
		VisualActive:     sec.visualActive,
		VisualAnchor:     sec.visualAnchor,
	}
}

func fromExplorerSectionData(data explorer.SectionData, vp viewport.Model) sectionState {
	return sectionState{
		rawLines:         data.RawLines,
		baseLines:        data.BaseLines,
		baseLineKinds:    data.BaseLineKinds,
		baseDisplayToRaw: data.BaseDisplayToRaw,
		viewLines:        data.ViewLines,
		viewLineKinds:    data.ViewLineKinds,
		displayToRaw:     data.DisplayToRaw,
		rawToDisplay:     data.RawToDisplay,
		hunkDisplayRange: data.HunkDisplayRange,
		changedDisplay:   data.ChangedDisplay,
		parsed:           data.Parsed,
		activeHunk:       data.ActiveHunk,
		activeLine:       data.ActiveLine,
		visualActive:     data.VisualActive,
		visualAnchor:     data.VisualAnchor,
		viewport:         vp,
	}
}
