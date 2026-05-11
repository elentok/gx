package status

import "github.com/elentok/gx/ui/explorer"

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

func toExplorerNavMode(mode navMode) explorer.NavMode {
	if mode == navLine {
		return explorer.NavLine
	}
	return explorer.NavHunk
}
