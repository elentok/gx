package log

import (
	"github.com/elentok/gx/ui/refrules"
)

// The important-refs/hide-refs matching logic lives in the ui/refrules package
// (shared with ui/commit so both views sort/filter decorations identically).
// These aliases keep the existing unexported names in this package.
type compiledRefRule = refrules.CompiledRefRule

var (
	compileHideRefs = refrules.CompileHideRefs
	isHiddenRef     = refrules.IsHiddenRef
	compileRefRules = refrules.CompileRefRules
	matchRefRule    = refrules.MatchRefRule
	sortDecorations = refrules.SortDecorations
)
