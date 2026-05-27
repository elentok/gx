package diffview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui/diffview/diffrender"
)

type VisibleDiffRow struct {
	DisplayIndex       int
	RawIndex           int
	Text               string
	Kind               diffrender.RowKind
	InActiveHunk       bool
	IsActiveRaw        bool
	IsActiveChangedRaw bool
	OverflowTop        bool
	OverflowBottom     bool
	IsSeparator        bool
}

func isSeparatorRow(text string, renderMode RenderMode) bool {
	if renderMode != RenderModeSideBySide {
		return false
	}
	return IsDeltaSectionDivider(strings.TrimSpace(ansi.Strip(text)))
}

