package explorer

// FocusPane identifies which pane currently owns explorer input.
type FocusPane int

const (
	FocusList FocusPane = iota
	FocusDiff
)

// Section identifies the primary or secondary diff section shown by an
// explorer host. Status maps these to unstaged/staged; commit can use only the
// primary section.
type Section int

const (
	SectionPrimary Section = iota
	SectionSecondary
)

// NavMode controls whether diff navigation targets hunks or changed lines.
type NavMode int

const (
	NavHunk NavMode = iota
	NavLine
)

// RenderMode controls how a diff section is rendered.
type RenderMode int

const (
	RenderUnified RenderMode = iota
	RenderSideBySide
)

// FileSelection captures the current file selection from an explorer host.
type FileSelection struct {
	Path       string
	RenameFrom string
	Untracked  bool
}

// DiffSelection captures the current diff selection from an explorer host.
type DiffSelection struct {
	File FileSelection
}
