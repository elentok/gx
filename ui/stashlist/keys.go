package stashlist

import "github.com/elentok/gx/ui/keys"

const (
	bindingStashHelp   keys.BindingID = "help"
	bindingStashBack   keys.BindingID = "back"
	bindingStashDown   keys.BindingID = "down"
	bindingStashUp     keys.BindingID = "up"
	bindingStashBottom keys.BindingID = "bottom"
	bindingStashOpen   keys.BindingID = "open"
	bindingStashApply  keys.BindingID = "apply"
	bindingStashPop    keys.BindingID = "pop"
	bindingStashDrop   keys.BindingID = "drop"
	bindingStashCreate keys.BindingID = "create"
)

// newStashManager builds the key manager for the stash list panel. Keys that
// involve split/detail routing (q, esc, f, t, h) are handled directly in
// handleKey before the manager processes the event; they appear here only so
// the help page lists them.
func newStashManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingStashHelp, Seq: []string{"?"}, Categories: []string{"Other"}, Title: "help"},
		{ID: bindingStashBack, Seq: []string{"q"}, Categories: []string{"Other"}, Title: "back"},
		{ID: bindingStashBack, Seq: []string{"esc"}, Categories: []string{}, Title: ""},

		{ID: bindingStashDown, Seq: []string{"j"}, Categories: []string{"Navigation"}, Title: "down", Display: "↓/j"},
		{ID: bindingStashDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
		{ID: bindingStashUp, Seq: []string{"k"}, Categories: []string{"Navigation"}, Title: "up", Display: "↑/k"},
		{ID: bindingStashUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
		{ID: bindingStashBottom, Seq: []string{"G"}, Categories: []string{"Navigation"}, Title: "bottom", Display: "G"},
		{ID: bindingStashBottom, Seq: []string{"shift+g"}, Categories: []string{}, Title: ""},
		{ID: bindingStashOpen, Seq: []string{"enter"}, Categories: []string{"Navigation"}, Title: "open stash"},
		{ID: bindingStashOpen, Seq: []string{"l"}, Categories: []string{}, Title: ""},

		{ID: bindingStashApply, Seq: []string{"a"}, Categories: []string{"Stash"}, Title: "apply stash"},
		{ID: bindingStashPop, Seq: []string{"p"}, Categories: []string{"Stash"}, Title: "pop stash"},
		{ID: bindingStashDrop, Seq: []string{"d"}, Categories: []string{"Stash"}, Title: "drop stash"},
		{ID: bindingStashCreate, Seq: []string{"s"}, Categories: []string{"Stash"}, Title: "create stash"},
	})
}
