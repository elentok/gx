package filetree

import "github.com/elentok/gx/ui/keybindings"

const (
	filetreeCat = "Filetree"

	BindingMoveDown    keybindings.BindingID = "move-down"
	BindingMoveUp      keybindings.BindingID = "move-up"
	BindingCollapse    keybindings.BindingID = "collapse"
	BindingExpand      keybindings.BindingID = "expand"
	BindingToggle      keybindings.BindingID = "toggle"
	BindingSearch      keybindings.BindingID = "search"
	BindingSearchNext  keybindings.BindingID = "search-next"
	BindingSearchPrev  keybindings.BindingID = "search-prev"
	BindingBack        keybindings.BindingID = "back"
	BindingPageDown    keybindings.BindingID = "page-down"
	BindingPageUp      keybindings.BindingID = "page-up"
	BindingToggleStage keybindings.BindingID = "toggle-stage"
	BindingDiscard     keybindings.BindingID = "discard"
)

var filetreeBindings = []keybindings.Binding{
	{ID: BindingMoveDown, Seq: []string{"j"}, Categories: []string{filetreeCat}, Title: "move down", Display: "↓/j"},
	{ID: BindingMoveUp, Seq: []string{"k"}, Categories: []string{filetreeCat}, Title: "move up", Display: "↑/k"},
	{ID: BindingCollapse, Seq: []string{"h"}, Categories: []string{filetreeCat}, Title: "collapse / go to parent", Display: "h/←"},
	{ID: BindingExpand, Seq: []string{"l"}, Categories: []string{filetreeCat}, Title: "expand / open", Display: "l/→"},
	{ID: BindingToggle, Seq: []string{"enter"}, Categories: []string{filetreeCat}, Title: "open / toggle dir"},
	{ID: BindingSearch, Seq: []string{"/"}, Categories: []string{"Search"}, Title: "search"},
	{ID: BindingSearchNext, Seq: []string{"n"}, Categories: []string{"Search"}, Title: "next match"},
	{ID: BindingSearchPrev, Seq: []string{"N"}, Categories: []string{"Search"}, Title: "prev match"},
	{ID: BindingMoveDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
	{ID: BindingMoveUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
	{ID: BindingCollapse, Seq: []string{"left"}, Categories: []string{}, Title: ""},
	{ID: BindingExpand, Seq: []string{"right"}, Categories: []string{}, Title: ""},
	{ID: BindingBack, Seq: []string{"q"}, Categories: []string{filetreeCat}, Title: "back", Display: "q/esc"},
	{ID: BindingBack, Seq: []string{"esc"}, Categories: []string{}, Title: ""},
	{ID: BindingPageDown, Seq: []string{"ctrl+d"}, Categories: []string{filetreeCat}, Title: "scroll page down"},
	{ID: BindingPageUp, Seq: []string{"ctrl+u"}, Categories: []string{filetreeCat}, Title: "scroll page up"},
	{ID: BindingToggleStage, Seq: []string{"space"}, Categories: []string{filetreeCat}, Title: "toggle stage"},
	{ID: BindingDiscard, Seq: []string{"d"}, Categories: []string{filetreeCat}, Title: "discard"},
}
