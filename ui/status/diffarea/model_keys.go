package diffarea

import "github.com/elentok/gx/ui/keys"

const (
	diffCat = "Diff"

	BindingMoveDown   keys.BindingID = "move-down"
	BindingMoveUp     keys.BindingID = "move-up"
	BindingScrollDown keys.BindingID = "scroll-down"
	BindingScrollUp   keys.BindingID = "scroll-up"
	BindingPageDown   keys.BindingID = "page-down"
	BindingPageUp     keys.BindingID = "page-up"
	BindingNavMode    keys.BindingID = "nav-mode"
	BindingVisual     keys.BindingID = "visual"
	BindingFullscreen keys.BindingID = "fullscreen"
	BindingWrap       keys.BindingID = "wrap"
	BindingSearchNext keys.BindingID = "search-next"
	BindingSearchPrev keys.BindingID = "search-prev"
	BindingBack       keys.BindingID = "back"
	BindingApply      keys.BindingID = "apply"
	BindingDiscard    keys.BindingID = "discard"
	BindingNextFile   keys.BindingID = "next-file"
	BindingPrevFile   keys.BindingID = "prev-file"
)

var diffBindings = []keys.Binding{
	{ID: BindingMoveDown, Seq: []string{"j"}, Categories: []string{diffCat}, Title: "move down", Display: "↓/j"},
	{ID: BindingMoveUp, Seq: []string{"k"}, Categories: []string{diffCat}, Title: "move up", Display: "↑/k"},
	{ID: BindingScrollDown, Seq: []string{"J"}, Categories: []string{diffCat}, Title: "scroll down"},
	{ID: BindingScrollUp, Seq: []string{"K"}, Categories: []string{diffCat}, Title: "scroll up"},
	{ID: BindingPageDown, Seq: []string{"ctrl+d"}, Categories: []string{diffCat}, Title: "half page down"},
	{ID: BindingPageUp, Seq: []string{"ctrl+u"}, Categories: []string{diffCat}, Title: "half page up"},
	{ID: BindingNavMode, Seq: []string{"a"}, Categories: []string{diffCat}, Title: "toggle hunk/line mode"},
	{ID: BindingVisual, Seq: []string{"v"}, Categories: []string{diffCat}, Title: "visual mode"},
	{ID: BindingFullscreen, Seq: []string{"F"}, Categories: []string{diffCat}, Title: "fullscreen"},
	{ID: BindingFullscreen, Seq: []string{"shift+f"}, Categories: []string{}, Title: ""},
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
