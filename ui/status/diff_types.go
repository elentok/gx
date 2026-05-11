package status

import "github.com/elentok/gx/ui/status/diffarea"

type focusPane int

const (
	focusFiletree focusPane = iota
	focusDiff
)

type diffSection = diffarea.Section

const (
	sectionUnstaged = diffarea.SectionUnstaged
	sectionStaged   = diffarea.SectionStaged
)
