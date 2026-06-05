package stashlist

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	stashpkg "github.com/elentok/gx/ui/stash"
)

type stashAutoReloadMsg struct{}
type stashApplyDoneMsg struct{ err error }
type stashPopDoneMsg   struct{ err error }
type stashDropDoneMsg  struct{ err error }

// AutoReload is called by the app shell when this tab is stale (gate epoch
// mismatch). It dispatches stashAutoReloadMsg so the list reload runs inside
// Update, preserving the selected entry index and split state.
func (m Model) AutoReload() tea.Cmd {
	return func() tea.Msg { return stashAutoReloadMsg{} }
}

func (m Model) cmdApply(ref string) tea.Cmd {
	root := m.worktreeRoot
	return func() tea.Msg {
		_, err := git.StashApply(root, ref)
		return stashApplyDoneMsg{err: err}
	}
}

func (m Model) cmdPopRef(ref string) tea.Cmd {
	root := m.worktreeRoot
	return func() tea.Msg {
		_, err := git.StashPopRef(root, ref)
		return stashPopDoneMsg{err: err}
	}
}

func (m Model) cmdDrop(ref string) tea.Cmd {
	root := m.worktreeRoot
	return func() tea.Msg {
		_, err := git.StashDrop(root, ref)
		return stashDropDoneMsg{err: err}
	}
}

func (m Model) handleApplyDone(msg stashApplyDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("stash apply: " + msg.err.Error())
	}
	return m, tea.Batch(notify.Success("applied stash"), m.stashList.cmdLoad(), nav.RepoMutated())
}

func (m Model) handlePopDone(msg stashPopDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("stash pop: " + msg.err.Error())
	}
	return m, tea.Batch(notify.Success("popped stash"), m.stashList.cmdLoad(), nav.RepoMutated())
}

func (m Model) handleDropDone(msg stashDropDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, notify.Error("stash drop: " + msg.err.Error())
	}
	return m, tea.Batch(notify.Success("dropped stash"), m.stashList.cmdLoad(), nav.RepoMutated())
}

func (m Model) handleStashCreateUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.stashCreate.Update(msg)
	m.stashCreate = next
	if !result.Done {
		return m, cmd
	}
	if result.Err != nil {
		return m, notify.Error("stash: " + result.Err.Error())
	}
	if result.Outcome == stashpkg.OutcomeStashed {
		return m, tea.Batch(notify.Success("stash created"), m.stashList.cmdLoad(), nav.RepoMutated())
	}
	return m, nil
}
