package tree

import "github.com/elentok/gx/ui/keys"

const (
	treeCat = "Tree"

	BindingMoveDown   keys.BindingID = "move-down"
	BindingMoveUp     keys.BindingID = "move-up"
	BindingCollapse   keys.BindingID = "collapse"
	BindingExpand     keys.BindingID = "expand"
	BindingToggle     keys.BindingID = "toggle"
	BindingSearch     keys.BindingID = "search"
	BindingSearchNext keys.BindingID = "search-next"
	BindingSearchPrev keys.BindingID = "search-prev"
	BindingBack       keys.BindingID = "back"
	BindingPageDown   keys.BindingID = "page-down"
	BindingPageUp     keys.BindingID = "page-up"
)

var treeBindings = []keys.Binding{
	{ID: BindingMoveDown, Seq: []string{"j"}, Categories: []string{treeCat}, Title: "move down", Display: "↓/j"},
	{ID: BindingMoveUp, Seq: []string{"k"}, Categories: []string{treeCat}, Title: "move up", Display: "↑/k"},
	{ID: BindingCollapse, Seq: []string{"h"}, Categories: []string{treeCat}, Title: "collapse / go to parent", Display: "h/←"},
	{ID: BindingExpand, Seq: []string{"l"}, Categories: []string{treeCat}, Title: "expand / open", Display: "l/→"},
	{ID: BindingToggle, Seq: []string{"enter"}, Categories: []string{treeCat}, Title: "open / toggle"},
	{ID: BindingSearch, Seq: []string{"/"}, Categories: []string{"Search"}, Title: "search"},
	{ID: BindingSearchNext, Seq: []string{"n"}, Categories: []string{"Search"}, Title: "next match"},
	{ID: BindingSearchPrev, Seq: []string{"N"}, Categories: []string{"Search"}, Title: "prev match"},
	{ID: BindingMoveDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
	{ID: BindingMoveUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
	{ID: BindingCollapse, Seq: []string{"left"}, Categories: []string{}, Title: ""},
	{ID: BindingExpand, Seq: []string{"right"}, Categories: []string{}, Title: ""},
	{ID: BindingBack, Seq: []string{"q"}, Categories: []string{treeCat}, Title: "back", Display: "q/esc"},
	{ID: BindingBack, Seq: []string{"esc"}, Categories: []string{}, Title: ""},
	{ID: BindingPageDown, Seq: []string{"ctrl+d"}, Categories: []string{treeCat}, Title: "scroll page down"},
	{ID: BindingPageUp, Seq: []string{"ctrl+u"}, Categories: []string{treeCat}, Title: "scroll page up"},
}
