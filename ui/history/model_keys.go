package history

import "github.com/elentok/gx/ui/keys"

// Binding IDs shared across the three pages' key managers. Reused IDs (e.g.
// bindingHistoryHelp) let help.BuildSections merge alternative sequences for
// the same action into one row.
const (
	bindingHistoryHelp        keys.BindingID = "help"
	bindingHistoryFilter      keys.BindingID = "filter"
	bindingHistoryDown        keys.BindingID = "down"
	bindingHistoryUp          keys.BindingID = "up"
	bindingHistoryOpen        keys.BindingID = "open"
	bindingHistoryQuit        keys.BindingID = "quit"
	bindingHistoryBack        keys.BindingID = "back"
	bindingHistoryGrep        keys.BindingID = "grep"
	bindingHistoryResume      keys.BindingID = "resume"
	bindingHistoryYank        keys.BindingID = "yank"
	bindingHistoryToggleScope keys.BindingID = "toggle-scope"
)

// newProjectsManager describes the projects page's real bindings (see
// handleProjectsKey).
func newProjectsManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingHistoryHelp, Seq: []string{"?"}, Categories: []string{"Other"}, Title: "help"},
		{ID: bindingHistoryFilter, Seq: []string{"/"}, Categories: []string{"Other"}, Title: "filter"},
		{ID: bindingHistoryDown, Seq: []string{"j"}, Categories: []string{"Navigation"}, Title: "down", Display: "↓/j"},
		{ID: bindingHistoryDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
		{ID: bindingHistoryUp, Seq: []string{"k"}, Categories: []string{"Navigation"}, Title: "up", Display: "↑/k"},
		{ID: bindingHistoryUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
		{ID: bindingHistoryOpen, Seq: []string{"enter"}, Categories: []string{"Navigation"}, Title: "open project"},
		{ID: bindingHistoryGrep, Seq: []string{"ctrl+f"}, Categories: []string{"Other"}, Title: "grep transcripts"},
		{ID: bindingHistoryQuit, Seq: []string{"q"}, Categories: []string{"Other"}, Title: "quit"},
	})
}

// newConversationsManager describes the conversations page's real bindings
// (see handleConversationsKey).
func newConversationsManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingHistoryHelp, Seq: []string{"?"}, Categories: []string{"Other"}, Title: "help"},
		{ID: bindingHistoryFilter, Seq: []string{"/"}, Categories: []string{"Other"}, Title: "filter"},
		{ID: bindingHistoryDown, Seq: []string{"j"}, Categories: []string{"Navigation"}, Title: "down", Display: "↓/j"},
		{ID: bindingHistoryDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
		{ID: bindingHistoryUp, Seq: []string{"k"}, Categories: []string{"Navigation"}, Title: "up", Display: "↑/k"},
		{ID: bindingHistoryUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
		{ID: bindingHistoryOpen, Seq: []string{"enter"}, Categories: []string{"Navigation"}, Title: "export + edit"},
		{ID: bindingHistoryResume, Seq: []string{"ctrl+r"}, Categories: []string{"Other"}, Title: "resume"},
		{ID: bindingHistoryYank, Seq: []string{"ctrl+y"}, Categories: []string{"Other"}, Title: "yank session id"},
		{ID: bindingHistoryGrep, Seq: []string{"ctrl+f"}, Categories: []string{"Other"}, Title: "grep transcripts"},
		{ID: bindingHistoryBack, Seq: []string{"esc"}, Categories: []string{"Other"}, Title: "back"},
	})
}

// newGrepManager describes the grep page's real bindings (see
// handleGrepKey). The filter is always focused on this page, so navigation
// only lists the arrow keys (j/k are ordinary query characters here).
func newGrepManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingHistoryHelp, Seq: []string{"?"}, Categories: []string{"Other"}, Title: "help"},
		{ID: bindingHistoryDown, Seq: []string{"down"}, Categories: []string{"Navigation"}, Title: "down"},
		{ID: bindingHistoryUp, Seq: []string{"up"}, Categories: []string{"Navigation"}, Title: "up"},
		{ID: bindingHistoryOpen, Seq: []string{"enter"}, Categories: []string{"Navigation"}, Title: "export + edit"},
		{ID: bindingHistoryResume, Seq: []string{"ctrl+r"}, Categories: []string{"Other"}, Title: "resume"},
		{ID: bindingHistoryYank, Seq: []string{"ctrl+y"}, Categories: []string{"Other"}, Title: "yank session id"},
		{ID: bindingHistoryToggleScope, Seq: []string{"ctrl+g"}, Categories: []string{"Other"}, Title: "toggle scope"},
		{ID: bindingHistoryBack, Seq: []string{"esc"}, Categories: []string{"Other"}, Title: "back"},
	})
}
