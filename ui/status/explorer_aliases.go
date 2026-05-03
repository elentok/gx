package status

import "github.com/elentok/gx/ui/explorer"

type focusPane = explorer.FocusPane

const (
	focusStatus = explorer.FocusList
	focusDiff   = explorer.FocusDiff
)

type diffSection = explorer.Section

const (
	sectionUnstaged = explorer.SectionPrimary
	sectionStaged   = explorer.SectionSecondary
)

type navMode = explorer.NavMode

const (
	navHunk = explorer.NavHunk
	navLine = explorer.NavLine
)

type diffRenderMode = explorer.RenderMode

const (
	renderUnified    = explorer.RenderUnified
	renderSideBySide = explorer.RenderSideBySide
)
