package ui

// IconSet provides semantic icon names with nerd-font and plain fallbacks.
type IconSet struct {
	Check        string
	Close        string
	Dash         string
	Branch       string
	Worktree     string
	FolderClosed string
	FolderOpen   string
	// TriangleCollapsed/TriangleExpanded mark a ticket/queue row with
	// children (ticket 10), deliberately distinct from the epic row's own
	// FolderClosed/FolderOpen glyphs. Collapsed points up and expanded points
	// right. Sized full-width rather than the "small triangle" Unicode
	// variants (▴/▸), which read as barely visible at normal terminal font
	// sizes; the nerd-font set uses Codicon's own triangle_up/triangle_right
	// glyphs for the same reason.
	TriangleCollapsed string
	TriangleExpanded  string
	FileModified      string
	FileAdded         string
	FileDeleted       string
	FileRenamed       string
	FileSymlink       string
	Ahead             string
	Behind            string
	Search            string
	Partial           string
	Staged            string
	Warning           string
	Info              string
	Dot               string
	Ellipsis          string
	CIRunning         string
	Commented         string
	Comment           string
	MarkerReady       string
	MarkerBlocked     string
	MarkerWaiting     string

	// CheckboxChecked/CheckboxUnchecked render the tickets tab's execution-
	// queue selection marker (ticket 04) on epic/ticket rows.
	CheckboxChecked   string
	CheckboxUnchecked string

	TicketDraft          string
	TicketOpen           string
	TicketClaimed        string
	TicketBlocked        string
	TicketNeedsInfo      string
	TicketNeedsAttention string
	TicketDone           string
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
			Check:             "✓",
			Close:             "✗",
			Dash:              "-",
			Branch:            "branch",
			Worktree:          "Worktree",
			FolderClosed:      "▸",
			FolderOpen:        "▾",
			TriangleCollapsed: "▲",
			TriangleExpanded:  "▶",
			FileModified:      "M",
			FileAdded:         "N",
			FileDeleted:       "D",
			FileRenamed:       "R",
			FileSymlink:       "L",
			Ahead:             "ahead",
			Behind:            "behind",
			Search:            "*",
			Partial:           "+",
			Staged:            "✓",
			Warning:           "⚠",
			Info:              "i",
			Dot:               "·",
			Ellipsis:          "...",
			CIRunning:         "⟳",
			Commented:         "o",
			Comment:           "c",
			MarkerReady:       "*",
			MarkerBlocked:     "!",
			MarkerWaiting:     "-",

			CheckboxChecked:   "[x]",
			CheckboxUnchecked: "[ ]",

			TicketDraft:          "~",
			TicketOpen:           "o",
			TicketClaimed:        "@",
			TicketBlocked:        "x",
			TicketNeedsInfo:      "?",
			TicketNeedsAttention: "!",
			TicketDone:           "d",
			TicketError:          "!!",
			TicketPaused:         "P",
		}
	}
	return IconSet{
		Check:             "",
		Close:             "󰅙",
		Dash:              "—",
		Branch:            "",
		Worktree:          "󰙅",
		FolderClosed:      "",
		FolderOpen:        "",
		TriangleCollapsed: "",
		TriangleExpanded:  "",
		FileModified:      "",
		FileAdded:         "",
		FileDeleted:       "",
		FileRenamed:       "󰁔",
		FileSymlink:       "󰌷",
		Ahead:             "",
		Behind:            "",
		Search:            "󰍉",
		Partial:           "",
		Staged:            "",
		Warning:           "",
		Info:              "",
		Dot:               "·",
		Ellipsis:          "…",
		CIRunning:         "⟳",
		Commented:         "◐",
		Comment:           "󰆈",
		MarkerReady:       "●",
		MarkerBlocked:     "●",
		MarkerWaiting:     "○",

		CheckboxChecked:   "󰄲",
		CheckboxUnchecked: "󰄱",

		TicketDraft:          "✎",
		TicketOpen:           "",
		TicketClaimed:        "",
		TicketBlocked:        "",
		TicketNeedsInfo:      "",
		TicketNeedsAttention: "",
		TicketDone:           "",
		TicketError:          "",
		TicketPaused:         "",
	}
}
