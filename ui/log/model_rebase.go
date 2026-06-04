package log

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

type rebaseConfirmKind int

const (
	rebaseConfirmNone     rebaseConfirmKind = iota
	rebaseConfirmStash                      // stash before rebase -i
	rebaseConfirmStashPop                   // pop stash after split rebase returns focus
)

type rebaseConfirmState struct {
	kind rebaseConfirmKind
	yes  bool
	hash string // rebase base hash; only set for rebaseConfirmStash
}

func (s rebaseConfirmState) isOpen() bool { return s.kind != rebaseConfirmNone }

type rebaseFinishedMsg struct {
	err      error
	splitApp string
}

type rebaseStashMsg struct {
	hash string
	err  error
}

type rebaseStashPopMsg struct{ err error }

func (m Model) startRebaseInteractive() (tea.Model, tea.Cmd) {
	cursor := m.list.Selected()

	next := cursor + 1
	for next < len(m.rows) && m.rows[next].kind != rowCommit {
		next++
	}
	if next >= len(m.rows) {
		return m, notify.Warning("rebase -i: no parent commit below selection")
	}

	hash := m.rows[next].commit.FullHash

	dirty, err := git.HasUnstagedChanges(m.worktreeRoot)
	if err != nil {
		return m, notify.Error("rebase -i: " + err.Error())
	}
	if dirty {
		m.rebaseConfirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true, hash: hash}
		return m, nil
	}

	return m, m.cmdRunRebaseInteractive(hash)
}

func (m Model) handleRebaseConfirmUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	nextYes, decided, accepted, handled := components.UpdateConfirm(kp, m.rebaseConfirm.yes)
	if !handled {
		return m, nil
	}
	if !decided {
		m.rebaseConfirm.yes = nextYes
		return m, nil
	}
	kind := m.rebaseConfirm.kind
	hash := m.rebaseConfirm.hash
	m.rebaseConfirm = rebaseConfirmState{}
	if !accepted {
		return m, nil
	}
	switch kind {
	case rebaseConfirmStash:
		root := m.worktreeRoot
		return m, tea.Batch(
			notify.Progress("rebase-stash", "stashing..."),
			func() tea.Msg {
				_, err := git.StashPushAuto(root)
				return rebaseStashMsg{hash: hash, err: err}
			},
		)
	case rebaseConfirmStashPop:
		root := m.worktreeRoot
		return m, tea.Batch(
			notify.Progress("rebase-stash-pop", "popping stash..."),
			func() tea.Msg {
				_, err := git.StashPop(root)
				return rebaseStashPopMsg{err: err}
			},
		)
	}
	return m, nil
}

func (m Model) rebaseConfirmView(width int) string {
	var prompt string
	switch m.rebaseConfirm.kind {
	case rebaseConfirmStash:
		prompt = "Stash unstaged changes before running git rebase -i?"
	case rebaseConfirmStashPop:
		prompt = "Rebase done. Pop the stash now?"
	}
	modalW := width / 2
	if modalW < 48 {
		modalW = 48
	}
	return components.RenderConfirmModal(prompt, m.rebaseConfirm.yes, ui.ColorYellow, ui.ColorGreen, ui.ColorRed, ui.ColorSubtle, modalW)
}

func (m Model) handleRebaseStash(msg rebaseStashMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, tea.Batch(notify.Close("rebase-stash"), notify.Error("stash failed: "+msg.err.Error()))
	}
	m.rebaseDidStash = true
	return m, tea.Batch(notify.Close("rebase-stash"), m.cmdRunRebaseInteractive(msg.hash))
}

func (m Model) cmdRunRebaseInteractive(hash string) tea.Cmd {
	return terminalrun.Command(m.worktreeRoot, m.settings.Terminal, "git", []string{"rebase", "-i", hash}, func(err error, splitApp string) tea.Msg {
		return rebaseFinishedMsg{err: err, splitApp: splitApp}
	})
}

func (m Model) handleRebaseFinished(msg rebaseFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil && msg.splitApp == "" {
		return m, tea.Batch(notify.Error("rebase -i: "+msg.err.Error()), m.cmdReload())
	}
	if m.rebaseDidStash && msg.splitApp == "" {
		// exec mode: rebase done, pop stash now
		m.rebaseDidStash = false
		root := m.worktreeRoot
		return m, tea.Batch(
			notify.Progress("rebase-stash-pop", "popping stash..."),
			m.cmdReload(),
			nav.RepoMutated(),
			func() tea.Msg {
				_, err := git.StashPop(root)
				return rebaseStashPopMsg{err: err}
			},
		)
	}
	// kitty/tmux: rebaseDidStash stays true; pop prompt fires on next FocusMsg.
	// Emit RepoMutated only on success; error in split-terminal mode may not have mutated.
	var mutatedCmd tea.Cmd
	if msg.err == nil {
		mutatedCmd = nav.RepoMutated()
	}
	return m, tea.Batch(mutatedCmd, m.cmdReload())
}

func (m Model) handleRebaseStashPop(msg rebaseStashPopMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, tea.Batch(notify.Close("rebase-stash-pop"), notify.Error("stash pop failed: "+msg.err.Error()))
	}
	return m, notify.Close("rebase-stash-pop")
}
