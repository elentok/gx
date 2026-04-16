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
	FileModified string
	FileAdded    string
	FileDeleted  string
	FileRenamed  string
	FileSymlink  string
	Ahead        string
	Behind       string
	Search       string
	Partial      string
	Staged       string
}

func Icons(useNerdFont bool) IconSet {
	if !useNerdFont {
		return IconSet{
			Check:        "✓",
			Close:        "✗",
			Dash:         "-",
			Branch:       "branch",
			Worktree:     "Worktree",
			FolderClosed: "▸",
			FolderOpen:   "▾",
			FileModified: "M",
			FileAdded:    "N",
			FileDeleted:  "D",
			FileRenamed:  "R",
			FileSymlink:  "L",
			Ahead:        "ahead",
			Behind:       "behind",
			Search:       "*",
			Partial:      "+",
			Staged:       "✓",
		}
	}
	return IconSet{
		Check:        "",
		Close:        "󰅙",
		Dash:         "—",
		Branch:       "",
		Worktree:     "󰙅",
		FolderClosed: "",
		FolderOpen:   "",
		FileModified: "",
		FileAdded:    "",
		FileDeleted:  "",
		FileRenamed:  "󰁔",
		FileSymlink:  "󰌷",
		Ahead:        "",
		Behind:       "",
		Search:       "󰍉",
		Partial:      "",
		Staged:       "",
	}
}
