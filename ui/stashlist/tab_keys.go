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
func (t Tab) dispatchBinding(id keys.BindingID, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch id {
	case bindingStashHelp:
		t.keys.Reset()
		t.help.Open(t.width, t.height)
		return t, nil
	case bindingStashDown, bindingStashUp, bindingStashBottom:
		return t.navigateList(msg)
	case bindingStashOpen:
		return t.routeKeyToSplit(tea.KeyPressMsg{Code: tea.KeyEnter})
	case bindingStashApply:
		if ref := t.stashList.SelectedRef(); ref != "" {
			return t, t.cmdApply(ref)
		}
		return t, nil
	case bindingStashPop:
		if ref := t.stashList.SelectedRef(); ref != "" {
			return t, t.cmdPopRef(ref)
		}
		return t, nil
	case bindingStashDrop:
		if ref := t.stashList.SelectedRef(); ref != "" {
			return t, t.cmdDrop(ref)
		}
		return t, nil
	case bindingStashCreate:
		cmd := t.stashCreate.Open(t.worktreeRoot, false)
		return t, cmd
	}
	return t, nil
}

func (t Tab) navigateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	prevRef := t.stashList.SelectedRef()
	updated, cmd := t.stashList.Update(msg)
	t.stashList = updated.(Model)
	t.split = t.split.WithListRef(t.stashList.SelectedRef())
	if ref := t.stashList.SelectedRef(); t.split.IsSplit() && ref != prevRef && ref != "" {
		t.commitDetail = t.commitDetail.WithRef(ref)
		t = t.syncPanelSizes()
	}
	return t, cmd
}
