// Package modal holds the dialogs drawn over the worktrees page. A modal owns
// only its own state; what it wants from the page comes back in a Result.
package modal

import (
	"github.com/elentok/gx/git"

	tea "charm.land/bubbletea/v2"
)

// Modal is one dialog drawn over the worktrees page. The page's modal is nil
// in normal mode. A modal owns only its own state; what it wants from the page
// (close, a spinner, another modal) comes back in a Result and the root
// applies it.
type Modal interface {
	Update(msg tea.Msg) (Modal, tea.Cmd, Result)
	// View renders the modal, centred over the page. An empty view draws
	// nothing.
	View(width, height int) string
	InputFocused() bool
}

// BottomAnchored marks a modal drawn at the bottom of the page (where
// settings.InputModalBottom puts it) instead of centred.
type BottomAnchored interface {
	BottomAnchored()
}

// Result is what a modal asks the root to do after an update.
type Result struct {
	// Unhandled sends the message on to the page's own handling (paste and
	// wheel events a modal ignores).
	Unhandled bool
	// Close dismisses the modal.
	Close bool
	// OpenLogs replaces the modal with the last job's output.
	OpenLogs bool
	// Spinner, when set, starts the busy spinner with this label.
	Spinner string
	// Action is what only the page can do after Close (read the cursor
	// worktree, move the table, open the next modal). Nil when there is none.
	Action Action
}

// Action is a closed set of requests a modal makes of the page. The root
// interprets each in applyModalAction.
type Action interface {
	modalAction()
}

// Clipboard holds the yanked file paths and their source worktree.
type Clipboard struct {
	SrcPath string   // absolute path of source worktree
	SrcName string   // display name
	Files   []string // relative file paths
}

type (
	// SearchQueryChanged recomputes the matches and jumps to the current one.
	SearchQueryChanged struct{}
	// SearchCursorMoved jumps to the search's current match.
	SearchCursorMoved struct{}
	// SearchClosed clears the search highlights.
	SearchClosed struct{}
	// PasteHere pastes the clipboard into the worktree under the cursor.
	PasteHere struct{ Clipboard Clipboard }
	// MoveCursor forwards a navigation key to the worktree table.
	MoveCursor struct{ Key tea.KeyPressMsg }
	// OpenPaste enters paste mode holding the clipboard.
	OpenPaste struct{ Clipboard Clipboard }
	// SubmitName runs the command for a name entered in a text prompt.
	SubmitName struct {
		Kind NameKind
		Name string
	}
	// Rebase rebases Wt onto Upstream.
	Rebase struct {
		Wt       git.Worktree
		Upstream string
		Log      string
	}
	// ForcePush force-pushes Wt.
	ForcePush struct {
		Wt  git.Worktree
		Log string
	}
)

func (SearchQueryChanged) modalAction() {}
func (SearchCursorMoved) modalAction()  {}
func (SearchClosed) modalAction()       {}
func (PasteHere) modalAction()          {}
func (MoveCursor) modalAction()         {}
func (OpenPaste) modalAction()          {}
func (SubmitName) modalAction()         {}
func (Rebase) modalAction()             {}
func (ForcePush) modalAction()          {}

// NameKind says which command a text prompt's name feeds.
type NameKind int

const (
	NameRename NameKind = iota
	NameClone
	NameNew
	NameNewAndOpen
)
