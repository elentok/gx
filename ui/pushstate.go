package ui

import (
	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/git"
)

var (
	PushStatePushedStyle     = lipgloss.NewStyle().Foreground(ColorGreen)
	PushStateUnpushedStyle   = lipgloss.NewStyle().Foreground(ColorOrange)
	PushStateDivergedStyle   = lipgloss.NewStyle().Foreground(ColorRed)
	PushStateRemoteOnlyStyle = lipgloss.NewStyle().Foreground(ColorMauve)
)

// PushState describes a commit's push/pull relationship to its upstream: an
// icon/style pair for compact rendering, and a textual label for contexts
// (like a single-commit header) that lack surrounding list context.
type PushState struct {
	Icon  string
	Label string
	Style lipgloss.Style
}

// CommitPushState classifies a commit's relationship to its upstream branch
// into a PushState. branchDiverged distinguishes a plain unpushed commit from
// one on a branch that has diverged from its upstream.
func CommitPushState(class git.BranchHistoryClass, branchDiverged bool) PushState {
	switch class {
	case git.BranchHistoryLocalOnly:
		if branchDiverged {
			return PushState{"󰃻", "diverged", PushStateDivergedStyle}
		}
		return PushState{"󰜷", "unpushed", PushStateUnpushedStyle}
	case git.BranchHistoryShared:
		return PushState{"✔", "pushed", PushStatePushedStyle}
	case git.BranchHistoryRemoteOnly:
		return PushState{"󰜮", "remote only", PushStateRemoteOnlyStyle}
	default:
		return PushState{" ", "", lipgloss.NewStyle()}
	}
}
