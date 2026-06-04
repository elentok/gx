package stashlist

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	stashpkg "github.com/elentok/gx/ui/stash"
)

type stashAutoReloadMsg struct{}
type stashApplyDoneMsg  struct{ err error }
type stashPopDoneMsg    struct{ err error }
type stashDropDoneMsg   struct{ err error }

// AutoReload is called by the app shell when this tab is stale (gate epoch
// mismatch). It dispatches stashAutoReloadMsg so the list reload runs inside
// Update, preserving the selected entry index and split state.
func (t Tab) AutoReload() tea.Cmd {
	return func() tea.Msg { return stashAutoReloadMsg{} }
}

func (t Tab) cmdApply(ref string) tea.Cmd {
	root := t.worktreeRoot
	return func() tea.Msg {
		_, err := git.StashApply(root, ref)
		return stashApplyDoneMsg{err: err}
	}
}

func (t Tab) cmdPopRef(ref string) tea.Cmd {
	root := t.worktreeRoot
	return func() tea.Msg {
		_, err := git.StashPopRef(root, ref)
		return stashPopDoneMsg{err: err}
	}
}

func (t Tab) cmdDrop(ref string) tea.Cmd {
	root := t.worktreeRoot
	return func() tea.Msg {
		_, err := git.StashDrop(root, ref)
		return stashDropDoneMsg{err: err}
	}
}

func (t Tab) handleApplyDone(msg stashApplyDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return t, notify.Error("stash apply: " + msg.err.Error())
	}
	return t, tea.Batch(notify.Success("applied stash"), t.stashList.cmdLoad(), nav.RepoMutated())
}

func (t Tab) handlePopDone(msg stashPopDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return t, notify.Error("stash pop: " + msg.err.Error())
	}
	return t, tea.Batch(notify.Success("popped stash"), t.stashList.cmdLoad(), nav.RepoMutated())
}

func (t Tab) handleDropDone(msg stashDropDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return t, notify.Error("stash drop: " + msg.err.Error())
	}
	return t, tea.Batch(notify.Success("dropped stash"), t.stashList.cmdLoad(), nav.RepoMutated())
}

func (t Tab) handleStashCreateUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := t.stashCreate.Update(msg)
	t.stashCreate = next
	if !result.Done {
		return t, cmd
	}
	if result.Err != nil {
		return t, notify.Error("stash: " + result.Err.Error())
	}
	if result.Outcome == stashpkg.OutcomeStashed {
		return t, tea.Batch(notify.Success("stash created"), t.stashList.cmdLoad(), nav.RepoMutated())
	}
	return t, nil
}
