package app

import "github.com/elentok/gx/ui/keys"

const (
	BindingPrevTab      keys.BindingID = "app-prev-tab"
	BindingNextTab      keys.BindingID = "app-next-tab"
	BindingGotoWorktree keys.BindingID = "app-goto-worktrees-tab"
	BindingGotoLog      keys.BindingID = "app-goto-log-tab"
	BindingGotoStatus   keys.BindingID = "app-goto-status-tab"
	BindingGotoStash    keys.BindingID = "app-goto-stash-tab"
	BindingGotoPRs      keys.BindingID = "app-goto-prs-tab"
	BindingGotoTickets  keys.BindingID = "app-goto-tickets-tab"
)

func Bindings() []keys.Binding {
	return []keys.Binding{
		{ID: BindingPrevTab, Seq: []string{"g", ","}, Categories: []string{"App"}, Title: "prev tab"},
		{ID: BindingNextTab, Seq: []string{"g", "."}, Categories: []string{"App"}, Title: "next tab"},
		// Number keys registered before their chord twins so help merges them
		// number-first (1/gw, not gw/1).
		{ID: BindingGotoWorktree, Seq: []string{"1"}, Categories: []string{"App"}, Title: "worktrees tab"},
		{ID: BindingGotoLog, Seq: []string{"2"}, Categories: []string{"App"}, Title: "log tab"},
		{ID: BindingGotoStatus, Seq: []string{"3"}, Categories: []string{"App"}, Title: "status tab"},
		{ID: BindingGotoStash, Seq: []string{"4"}, Categories: []string{"App"}, Title: "stash tab"},
		{ID: BindingGotoPRs, Seq: []string{"5"}, Categories: []string{"App"}, Title: "PRs tab"},
		{ID: BindingGotoTickets, Seq: []string{"6"}, Categories: []string{"App"}, Title: "tickets tab"},
		{ID: BindingGotoWorktree, Seq: []string{"g", "w"}, Categories: []string{"App"}, Title: "worktrees tab"},
		{ID: BindingGotoLog, Seq: []string{"g", "l"}, Categories: []string{"App"}, Title: "log tab"},
		{ID: BindingGotoStatus, Seq: []string{"g", "s"}, Categories: []string{"App"}, Title: "status tab"},
		{ID: BindingGotoStash, Seq: []string{"g", "S"}, Categories: []string{"App"}, Title: "stash tab"},
		{ID: BindingGotoPRs, Seq: []string{"g", "p"}, Categories: []string{"App"}, Title: "PRs tab"},
		{ID: BindingGotoTickets, Seq: []string{"g", "t"}, Categories: []string{"App"}, Title: "tickets tab"},
	}
}

func hintsForPrefix(prefix string) []keys.Binding {
	if prefix != "g" {
		return nil
	}
	return []keys.Binding{
		{Seq: []string{","}, Title: "prev tab"},
		{Seq: []string{"."}, Title: "next tab"},
		{Seq: []string{"w"}, Title: "worktrees tab"},
		{Seq: []string{"l"}, Title: "log tab"},
		{Seq: []string{"s"}, Title: "status tab"},
		{Seq: []string{"S"}, Title: "stash tab"},
		{Seq: []string{"p"}, Title: "PRs tab"},
		{Seq: []string{"t"}, Title: "tickets tab"},
	}
}
