package diffarea

import "github.com/elentok/gx/ui/keybindings"

const (
	diffCat = "Diff"

	BindingMoveDown   keybindings.BindingID = "move-down"
	BindingMoveUp     keybindings.BindingID = "move-up"
	BindingScrollDown keybindings.BindingID = "scroll-down"
	BindingScrollUp   keybindings.BindingID = "scroll-up"
	BindingPageDown   keybindings.BindingID = "page-down"
	BindingPageUp     keybindings.BindingID = "page-up"
	BindingNavMode    keybindings.BindingID = "nav-mode"
	BindingVisual     keybindings.BindingID = "visual"
	BindingFullscreen keybindings.BindingID = "fullscreen"
	BindingWrap       keybindings.BindingID = "wrap"
	BindingSearchNext keybindings.BindingID = "search-next"
	BindingSearchPrev keybindings.BindingID = "search-prev"
	BindingBack       keybindings.BindingID = "back"
	BindingApply      keybindings.BindingID = "apply"
	BindingDiscard    keybindings.BindingID = "discard"
	BindingNextFile   keybindings.BindingID = "next-file"
	BindingPrevFile   keybindings.BindingID = "prev-file"
)

var diffBindings = []keybindings.Binding{
	{ID: BindingMoveDown, Seq: []string{"j"}, Categories: []string{diffCat}, Title: "move down", Display: "↓/j"},
	{ID: BindingMoveUp, Seq: []string{"k"}, Categories: []string{diffCat}, Title: "move up", Display: "↑/k"},
	{ID: BindingScrollDown, Seq: []string{"J"}, Categories: []string{diffCat}, Title: "scroll down"},
	{ID: BindingScrollUp, Seq: []string{"K"}, Categories: []string{diffCat}, Title: "scroll up"},
	{ID: BindingPageDown, Seq: []string{"ctrl+d"}, Categories: []string{diffCat}, Title: "half page down"},
	{ID: BindingPageUp, Seq: []string{"ctrl+u"}, Categories: []string{diffCat}, Title: "half page up"},
	{ID: BindingNavMode, Seq: []string{"a"}, Categories: []string{diffCat}, Title: "toggle hunk/line mode"},
	{ID: BindingVisual, Seq: []string{"v"}, Categories: []string{diffCat}, Title: "visual mode"},
	{ID: BindingFullscreen, Seq: []string{"f"}, Categories: []string{diffCat}, Title: "fullscreen"},
	{ID: BindingWrap, Seq: []string{"w"}, Categories: []string{diffCat}, Title: "soft wrap"},
	{ID: BindingSearchNext, Seq: []string{"n"}, Categories: []string{"Search"}, Title: "next match"},
	{ID: BindingSearchPrev, Seq: []string{"N"}, Categories: []string{"Search"}, Title: "prev match"},
	{ID: BindingMoveDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
	{ID: BindingMoveUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
	{ID: BindingBack, Seq: []string{"esc"}, Categories: []string{diffCat}, Title: "back to filetree", Display: "esc/q/h/←"},
	{ID: BindingBack, Seq: []string{"q"}, Categories: []string{}, Title: ""},
	{ID: BindingBack, Seq: []string{"h"}, Categories: []string{}, Title: ""},
	{ID: BindingBack, Seq: []string{"left"}, Categories: []string{}, Title: ""},
	{ID: BindingApply, Seq: []string{"space"}, Categories: []string{diffCat}, Title: "apply selection"},
	{ID: BindingDiscard, Seq: []string{"d"}, Categories: []string{diffCat}, Title: "discard"},
	{ID: BindingNextFile, Seq: []string{"."}, Categories: []string{diffCat}, Title: "next file"},
	{ID: BindingPrevFile, Seq: []string{","}, Categories: []string{diffCat}, Title: "prev file"},
}
