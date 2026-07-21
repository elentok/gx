package prs

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/keys"
)

const (
	bindingPRsHelp keys.BindingID = "help"
	bindingPRsBack keys.BindingID = "back"
)

// newPRsManager builds the key manager for the PRs tab.
func newPRsManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingPRsHelp, Seq: []string{"?"}, Categories: []string{"Other"}, Title: "help"},
		{ID: bindingPRsBack, Seq: []string{"q"}, Categories: []string{"Other"}, Title: "back"},
		{ID: bindingPRsBack, Seq: []string{"esc"}, Categories: []string{}, Title: ""},
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
	}
	return m, nil
}
