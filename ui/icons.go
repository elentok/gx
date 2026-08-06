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

	// CheckboxChecked/CheckboxUnchecked render the tickets tab's execution-
	// queue selection marker (ticket 04) on epic/ticket rows.
	CheckboxChecked   string
	CheckboxUnchecked string

	TicketOpen           string
	TicketClaimed        string
	TicketBlocked        string
	TicketNeedsInfo      string
	TicketNeedsAttention string
	TicketDone           string
	TicketSuperseded     string
	TicketError          string
	// TicketPaused is a live ralph-loop orchestrator state (an iteration
	// paused mid-run, e.g. smart-zone/rate-limit), distinct from
	// TicketBlocked's ticket-graph "blocked by" state — see ui/tickets'
	// statusIconAndStyle vs. ralph-loop's live row rendering.
	TicketPaused string
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

			CheckboxChecked:   "[x]",
			CheckboxUnchecked: "[ ]",

			TicketOpen:           "o",
			TicketClaimed:        "@",
			TicketBlocked:        "x",
			TicketNeedsInfo:      "?",
			TicketNeedsAttention: "!",
			TicketDone:           "d",
			TicketSuperseded:     "s",
			TicketError:          "!!",
			TicketPaused:         "P",
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

		CheckboxChecked:   "󰄲",
		CheckboxUnchecked: "󰄱",

		TicketOpen:           "",
		TicketClaimed:        "",
		TicketBlocked:        "",
		TicketNeedsInfo:      "",
		TicketNeedsAttention: "",
		TicketDone:           "",
		TicketSuperseded:     "",
		TicketError:          "",
		TicketPaused:         "",
	}
}
