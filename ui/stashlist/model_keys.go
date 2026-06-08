package stashlist

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/keys"
)

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

// dispatchBinding runs the action for a resolved stash-list binding. The
// original key message is forwarded for navigation bindings so the stash list
// can distinguish j/k/G variants.
func (m Model) dispatchBinding(id keys.BindingID, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch id {
	case bindingStashHelp:
		m.keys.Reset()
		m.help.Open(m.width, m.height)
		return m, nil
	case bindingStashDown, bindingStashUp, bindingStashBottom:
		return m.navigateList(msg)
	case bindingStashOpen:
		return m.routeKeyToSplit(tea.KeyPressMsg{Code: tea.KeyEnter})
	case bindingStashApply:
		if ref := m.stashList.SelectedRef(); ref != "" {
			return m, m.cmdApply(ref)
		}
		return m, nil
	case bindingStashPop:
		if ref := m.stashList.SelectedRef(); ref != "" {
			return m, m.cmdPopRef(ref)
		}
		return m, nil
	case bindingStashDrop:
		if ref := m.stashList.SelectedRef(); ref != "" {
			return m, m.cmdDrop(ref)
		}
		return m, nil
	case bindingStashCreate:
		cmd := m.stashCreate.Open(m.worktreeRoot, false)
		return m, cmd
	}
	return m, nil
}

func (m Model) navigateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	prevRef := m.stashList.SelectedRef()
	updated, cmd := m.stashList.Update(msg)
	m.stashList = updated.(listPanel)
	m.split = m.split.WithListRef(m.stashList.SelectedRef())
	if ref := m.stashList.SelectedRef(); m.split.IsSplit() && ref != prevRef && ref != "" {
		var refCmd tea.Cmd
		m.commitDetail, refCmd = m.commitDetail.WithRef(ref)
		m = m.syncPanelSizes()
		return m, tea.Batch(cmd, refCmd)
	}
	return m, cmd
}
