package status

import (
	"charm.land/bubbles/v2/viewport"

	"github.com/elentok/gx/ui/explorer"
)

type sectionState struct {
	data     explorer.SectionData
	viewport viewport.Model
	// colorized indicates whether this section has delta-colored content.
	// When false in unified mode, the view reserves gutter width to keep
	// alignment with colored rendering.
	colorized bool
}

type flashState struct {
	active  bool
	section diffSection
	navMode navMode
	hunk    int
	line    int
	frames  int
}

type diffInteractionState struct {
	focus          focusPane
	section        diffSection
	navMode        navMode
	renderMode     diffRenderMode
	diffFullscreen bool
	wrapSoft       bool
	unstaged       sectionState
	staged         sectionState
	flash          flashState
}

func newSectionState() sectionState {
	vp := viewport.New()
	return sectionState{
		data:     explorer.NewSectionData(),
		viewport: vp,
	}
}
