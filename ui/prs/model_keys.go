package prs

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

const (
	bindingPRsHelp        keys.BindingID = "help"
	bindingPRsBack        keys.BindingID = "back"
	bindingPRsDown        keys.BindingID = "down"
	bindingPRsUp          keys.BindingID = "up"
	bindingPRsOpen        keys.BindingID = "open"
	bindingPRsRefresh     keys.BindingID = "refresh"
	bindingPRsRefreshMenu keys.BindingID = "refresh-menu"
)

// newPRsManager builds the key manager for the PRs tab.
func newPRsManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingPRsHelp, Seq: []string{"?"}, Categories: []string{"Other"}, Title: "help"},
		{ID: bindingPRsBack, Seq: []string{"q"}, Categories: []string{"Other"}, Title: "back"},
		{ID: bindingPRsBack, Seq: []string{"esc"}, Categories: []string{}, Title: ""},
		{ID: bindingPRsDown, Seq: []string{"j"}, Categories: []string{"Navigation"}, Title: "down", Display: "↓/j"},
		{ID: bindingPRsDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
		{ID: bindingPRsUp, Seq: []string{"k"}, Categories: []string{"Navigation"}, Title: "up", Display: "↑/k"},
		{ID: bindingPRsUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
		{ID: bindingPRsOpen, Seq: []string{"enter"}, Categories: []string{"Navigation"}, Title: "open in browser"},
		{ID: bindingPRsOpen, Seq: []string{"o"}, Categories: []string{}, Title: ""},
		{ID: bindingPRsRefresh, Seq: []string{"R"}, Categories: []string{"Other"}, Title: "refresh"},
		{ID: bindingPRsRefreshMenu, Seq: []string{"m", "r"}, Categories: []string{"Global"}, Title: "refresh"},
	})
}

// dispatchBinding runs the action for a resolved PRs-tab binding.
func (m Model) dispatchBinding(id keys.BindingID, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch id {
	case bindingPRsHelp:
		m.keys.Reset()
		m.help.Open(m.width, m.height)
		return m, nil
	case bindingPRsBack:
		return m, nil
	case bindingPRsDown:
		m.list.Navigate(1, len(m.prs), m.visibleH())
		return m, nil
	case bindingPRsUp:
		m.list.Navigate(-1, len(m.prs), m.visibleH())
		return m, nil
	case bindingPRsOpen:
		return m, m.cmdOpenSelected()
	case bindingPRsRefresh, bindingPRsRefreshMenu:
		return m, tea.Batch(notify.Success("refreshed"), m.cmdLoad())
	}
	return m, nil
}
