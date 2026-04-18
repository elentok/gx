package worktrees

import "github.com/elentok/gx/ui"

type uiIcons struct {
	worktreePrefix string
	mainPrefix     string
	branchPrefix   string
	worktreeTitle  string
	aheadTitle     string
	behindTitle    string
	baseTitle      string
	checkmark      string
	x              string
	changesTitle   string
	dash           string
	ahead          string
	behind         string
}

func icons(useNerdFont bool) uiIcons {
	shared := ui.Icons(useNerdFont)
	if !useNerdFont {
		return uiIcons{
			worktreeTitle: shared.Worktree,
			aheadTitle:    "Commits ahead of remote",
			behindTitle:   "Commits behind remote",
			baseTitle:     "Base",
			checkmark:     shared.Check,
			x:             shared.Close,
			changesTitle:  "Changes",
			dash:          shared.Dash,
		}
	}
	return uiIcons{
		worktreePrefix: "󰉖 ",
		mainPrefix:     "󰋜 ",
		branchPrefix:   shared.Branch + " ",
		worktreeTitle:  shared.Worktree + " Worktree",
		aheadTitle:     " Commits ahead of remote",
		behindTitle:    " Commits behind remote",
		ahead:          shared.Ahead,
		behind:         shared.Behind,
		baseTitle:      "󰋜 Base",
		checkmark:      shared.Check,
		x:              shared.Close,
		changesTitle:   "󰈔 Changes",
		dash:           shared.Dash,
	}
}
