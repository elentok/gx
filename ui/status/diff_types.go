package status

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
