package status

import (
	"charm.land/bubbles/v2/viewport"

	"github.com/elentok/gx/ui/diffview"
)

type sectionState struct {
	data     diffview.DiffBuffer
	viewport viewport.Model
	// colorized indicates whether this section has delta-colored content.
	// When false in unified mode, the view reserves gutter width to keep
	// alignment with colored rendering.
	colorized bool
}

type flashState struct {
	active  bool
	section diffSection
	navMode diffview.NavMode
	hunk    int
	line    int
	frames  int
}

type diffArea struct {
	section        diffSection
	navMode        diffview.NavMode
	renderMode     diffview.RenderMode
	diffFullscreen bool
	wrapSoft       bool
	unstaged       sectionState
	staged         sectionState
	unstagedModel  diffview.Model
	stagedModel    diffview.Model
	flash          flashState
}

func newDiffArea() diffArea {
	area := diffArea{
		section:       sectionUnstaged,
		navMode:       diffview.NavModeHunk,
		renderMode:    diffview.RenderModeUnified,
		wrapSoft:      true,
		unstaged:      newSectionState(),
		staged:        newSectionState(),
		unstagedModel: diffview.NewModel(),
		stagedModel:   diffview.NewModel(),
	}
	area.applyModes()
	return area
}

func (d *diffArea) applyModes() {
	d.unstagedModel.SetRenderMode(d.renderMode)
	d.stagedModel.SetRenderMode(d.renderMode)
	d.unstagedModel.SetNavMode(d.navMode)
	d.stagedModel.SetNavMode(d.navMode)
	d.unstagedModel.EnableWrap(d.wrapSoft)
	d.stagedModel.EnableWrap(d.wrapSoft)
}

func newSectionState() sectionState {
	vp := viewport.New()
	return sectionState{
		data:     diffview.NewDiffBuffer(),
		viewport: vp,
	}
}
