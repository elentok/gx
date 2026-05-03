package status

import (
	"charm.land/bubbles/v2/viewport"

	"github.com/elentok/gx/ui/diff"
)

// These types model the reusable diff explorer core that both live status and
// historical commit review will eventually share.

type focusPane int

const (
	focusStatus focusPane = iota
	focusDiff
)

type diffSection int

const (
	sectionUnstaged diffSection = iota
	sectionStaged
)

type navMode int

const (
	navHunk navMode = iota
	navLine
)

type diffRenderMode int

const (
	renderUnified diffRenderMode = iota
	renderSideBySide
)

type diffDisplayRowKind = diff.RowKind

const (
	diffRowPlain      = diff.RowPlain
	diffRowAdded      = diff.RowAdded
	diffRowRemoved    = diff.RowRemoved
	diffRowHunkHeader = diff.RowHunkHeader
)

type sectionState struct {
	rawLines         []string
	baseLines        []string
	baseLineKinds    []diffDisplayRowKind
	baseDisplayToRaw []int
	viewLines        []string
	viewLineKinds    []diffDisplayRowKind
	displayToRaw     []int
	rawToDisplay     []int
	hunkDisplayRange [][2]int
	changedDisplay   []int
	parsed           diff.ParsedDiff
	activeHunk       int
	activeLine       int
	visualActive     bool
	visualAnchor     int
	viewport         viewport.Model
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
		activeHunk:   -1,
		activeLine:   -1,
		visualAnchor: -1,
		viewport:     vp,
	}
}
