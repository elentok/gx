package diffview

import "github.com/elentok/gx/ui/diffview/diffrender"

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
}

