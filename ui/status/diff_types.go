package status

import (
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/explorer"
)

type focusPane int

const (
	focusFiletree focusPane = iota
	focusDiff
)

type diffSection int

const (
	sectionUnstaged diffSection = iota
	sectionStaged
)

type navMode = diffview.NavMode

const (
	navHunk = diffview.NavModeHunk
	navLine = diffview.NavModeLine
)

type diffRenderMode = diffview.RenderMode

const (
	renderUnified    = diffview.RenderModeUnified
	renderSideBySide = diffview.RenderModeSideBySide
)

func toExplorerNavMode(mode navMode) explorer.NavMode {
	if mode == navLine {
		return explorer.NavLine
	}
	return explorer.NavHunk
}
