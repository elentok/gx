package diffarea

import "github.com/elentok/gx/ui/keys"

const (
	diffCat = "Diff"

	BindingFullscreen keys.BindingID = "fullscreen"
	BindingSearchNext keys.BindingID = "search-next"
	BindingSearchPrev keys.BindingID = "search-prev"
	BindingBack       keys.BindingID = "back"
	BindingApply      keys.BindingID = "apply"
	BindingDiscard    keys.BindingID = "discard"
	BindingNextFile   keys.BindingID = "next-file"
	BindingPrevFile   keys.BindingID = "prev-file"
)

var diffBindings = []keys.Binding{
	{ID: BindingFullscreen, Seq: []string{"f"}, Categories: []string{diffCat}, Title: "fullscreen"},
	{ID: BindingSearchNext, Seq: []string{"n"}, Categories: []string{"Search"}, Title: "next match"},
	{ID: BindingSearchPrev, Seq: []string{"N"}, Categories: []string{"Search"}, Title: "prev match"},
	{ID: BindingBack, Seq: []string{"esc"}, Categories: []string{diffCat}, Title: "back to filetree", Display: "esc/q/h/←"},
	{ID: BindingBack, Seq: []string{"q"}, Categories: []string{}, Title: ""},
	{ID: BindingBack, Seq: []string{"h"}, Categories: []string{}, Title: ""},
	{ID: BindingBack, Seq: []string{"left"}, Categories: []string{}, Title: ""},
	{ID: BindingApply, Seq: []string{"space"}, Categories: []string{diffCat}, Title: "apply selection"},
	{ID: BindingDiscard, Seq: []string{"d"}, Categories: []string{diffCat}, Title: "discard"},
	{ID: BindingNextFile, Seq: []string{"."}, Categories: []string{diffCat}, Title: "next file"},
	{ID: BindingPrevFile, Seq: []string{","}, Categories: []string{diffCat}, Title: "prev file"},
}
