package ui

// IconSet provides semantic icon names with nerd-font and plain fallbacks.
type IconSet struct {
	Check         string
	Close         string
	Dash          string
	Branch        string
	Worktree      string
	FolderClosed  string
	FolderOpen    string
	FileModified  string
	FileAdded     string
	FileDeleted   string
	FileRenamed   string
	FileSymlink   string
	Ahead         string
	Behind        string
	Search        string
	Partial       string
	Staged        string
	Warning       string
	Info          string
	Dot           string
	Ellipsis      string
	CIRunning     string
	Commented     string
	Comment       string
	MarkerReady   string
	MarkerBlocked string
	MarkerWaiting string

	TicketOpen      string
	TicketClaimed   string
	TicketBlocked   string
	TicketNeedsInfo string
	TicketDone      string
	TicketError     string
}

func Icons(useNerdFont bool) IconSet {
	if !useNerdFont {
		return IconSet{
			Check:         "✓",
			Close:         "✗",
			Dash:          "-",
			Branch:        "branch",
			Worktree:      "Worktree",
			FolderClosed:  "▸",
			FolderOpen:    "▾",
			FileModified:  "M",
			FileAdded:     "N",
			FileDeleted:   "D",
			FileRenamed:   "R",
			FileSymlink:   "L",
			Ahead:         "ahead",
			Behind:        "behind",
			Search:        "*",
			Partial:       "+",
			Staged:        "✓",
			Warning:       "⚠",
			Info:          "i",
			Dot:           "·",
			Ellipsis:      "...",
			CIRunning:     "⟳",
			Commented:     "o",
			Comment:       "c",
			MarkerReady:   "*",
			MarkerBlocked: "!",
			MarkerWaiting: "-",

			TicketOpen:      "o",
			TicketClaimed:   "@",
			TicketBlocked:   "x",
			TicketNeedsInfo: "?",
			TicketDone:      "d",
			TicketError:     "!!",
		}
	}
	return IconSet{
		Check:         "",
		Close:         "󰅙",
		Dash:          "—",
		Branch:        "",
		Worktree:      "󰙅",
		FolderClosed:  "",
		FolderOpen:    "",
		FileModified:  "",
		FileAdded:     "",
		FileDeleted:   "",
		FileRenamed:   "󰁔",
		FileSymlink:   "󰌷",
		Ahead:         "",
		Behind:        "",
		Search:        "󰍉",
		Partial:       "",
		Staged:        "",
		Warning:       "",
		Info:          "",
		Dot:           "·",
		Ellipsis:      "…",
		CIRunning:     "⟳",
		Commented:     "◐",
		Comment:       "󰆈",
		MarkerReady:   "●",
		MarkerBlocked: "●",
		MarkerWaiting: "○",

		TicketOpen:      "○",
		TicketClaimed:   "󰀄",
		TicketBlocked:   "󰦞",
		TicketNeedsInfo: "󰋗",
		TicketDone:      "󰄬",
		TicketError:     "󰀪",
	}
}
