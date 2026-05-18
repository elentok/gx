package app

import "github.com/elentok/gx/ui/keys"

const (
	BindingPrevTab      keys.BindingID = "app-prev-tab"
	BindingNextTab      keys.BindingID = "app-next-tab"
	BindingGotoWorktree keys.BindingID = "app-goto-worktrees-tab"
	BindingGotoLog      keys.BindingID = "app-goto-log-tab"
	BindingGotoStatus   keys.BindingID = "app-goto-status-tab"
)

func Bindings() []keys.Binding {
	return []keys.Binding{
		{ID: BindingPrevTab, Seq: []string{"g", ","}, Categories: []string{"App"}, Title: "prev tab"},
		{ID: BindingNextTab, Seq: []string{"g", "."}, Categories: []string{"App"}, Title: "next tab"},
		{ID: BindingGotoWorktree, Seq: []string{"g", "w"}, Categories: []string{"App"}, Title: "worktrees tab"},
		{ID: BindingGotoLog, Seq: []string{"g", "l"}, Categories: []string{"App"}, Title: "log tab"},
		{ID: BindingGotoStatus, Seq: []string{"g", "s"}, Categories: []string{"App"}, Title: "status tab"},
		{ID: BindingGotoWorktree, Seq: []string{"1"}, Categories: []string{"App"}, Title: "worktrees tab"},
		{ID: BindingGotoLog, Seq: []string{"2"}, Categories: []string{"App"}, Title: "log tab"},
		{ID: BindingGotoStatus, Seq: []string{"3"}, Categories: []string{"App"}, Title: "status tab"},
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
	}
}
