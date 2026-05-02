package log

import "charm.land/bubbles/v2/key"

var (
	logKeyTop         = key.NewBinding(key.WithKeys("gg"), key.WithHelp("gg", "top"))
	logKeyWorktrees   = key.NewBinding(key.WithKeys("gw"), key.WithHelp("gw", "goto worktrees"))
	logKeyGotoLog     = key.NewBinding(key.WithKeys("gl"), key.WithHelp("gl", "goto log"))
	logKeyStatus      = key.NewBinding(key.WithKeys("gs"), key.WithHelp("gs", "goto status"))
	logKeyHead        = key.NewBinding(key.WithKeys("gh"), key.WithHelp("gh", "goto HEAD"))
	logKeySearchNext  = key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "next"))
	logKeySearchPrev  = key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "prev"))
	logKeySearchClose = key.NewBinding(key.WithKeys("esc", "enter"), key.WithHelp("esc/enter", "close"))
)
