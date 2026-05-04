package log

import "charm.land/bubbles/v2/key"

var (
	logKeyUp         = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	logKeyDown       = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	logKeyTop        = key.NewBinding(key.WithKeys("gg"), key.WithHelp("gg", "top"))
	logKeyBottom     = key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom"))
	logKeyHead       = key.NewBinding(key.WithKeys("gh"), key.WithHelp("gh", "goto HEAD"))
	logKeyOpen       = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open commit"))
	logKeySearch     = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
	logKeyResultNext = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next result"))
	logKeyResultPrev = key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev result"))
	logKeyNextTag    = key.NewBinding(key.WithKeys("]t"), key.WithHelp("]t", "next tag"))
	logKeyPrevTag    = key.NewBinding(key.WithKeys("[t"), key.WithHelp("[t", "prev tag"))
	logKeyWorktrees  = key.NewBinding(key.WithKeys("gw"), key.WithHelp("gw", "goto worktrees"))
	logKeyGotoLog    = key.NewBinding(key.WithKeys("gl"), key.WithHelp("gl", "goto log"))
	logKeyStatus     = key.NewBinding(key.WithKeys("gs"), key.WithHelp("gs", "goto status"))
	logKeyBack       = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "back"))
	logKeyReload     = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload"))
	logKeyHelp       = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))

	logKeySearchNext  = key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "next"))
	logKeySearchPrev  = key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "prev"))
	logKeySearchClose = key.NewBinding(key.WithKeys("esc", "enter"), key.WithHelp("esc/enter", "close"))
)
