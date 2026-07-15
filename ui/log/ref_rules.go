package log

import (
	"github.com/elentok/gx/ui"
)

// The important-refs/hide-refs matching logic lives in the ui package
// (shared with ui/commit so both views sort/filter decorations identically).
// These aliases keep the existing unexported names in this package.
type compiledRefRule = ui.CompiledRefRule

var (
	compileHideRefs = ui.CompileHideRefs
	isHiddenRef     = ui.IsHiddenRef
	compileRefRules = ui.CompileRefRules
	matchRefRule    = ui.MatchRefRule
	sortDecorations = ui.SortDecorations
)
