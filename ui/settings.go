package ui

import (
	"github.com/elentok/gx/config"
)

// Settings holds configuration shared across all views.
type Settings struct {
	UseNerdFontIcons bool
	ImageDiffs       bool // used by the status diff view
	InputModalBottom config.InputModalBottom
	Terminal         Terminal
	EnableNavigation bool
	DiffContextLines int               // used by the status diff view
	NameAliases      map[string]string // used by the worktrees view
	LogConfig        config.LogConfig
}
