package status

import (
	"charm.land/bubbles/v2/viewport"

	"github.com/elentok/gx/ui/explorer"
)

type sectionState struct {
	data     explorer.SectionData
	viewport viewport.Model
	// colorized is true once the async delta colorization has arrived.
	// While false, the view adds left padding to reserve space for the delta
	// line-number gutter that will appear once colorization completes.
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

type explorerState struct {
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
