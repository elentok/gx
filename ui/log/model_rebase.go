package log

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
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

// rebaseFlow is the rebase -i state: the stash confirm modal and whether a
// stash was pushed before the rebase. Methods take the worktree root (and
// terminal where a command runs) from the caller.
type rebaseFlow struct {
	confirm rebaseConfirmState // confirm modal for stash-before-rebase and stash-pop-after-rebase
	// didStash: stash was pushed before rebase; pop prompt fires on FocusMsg
	// (kitty/tmux) or immediately (exec).
	didStash bool
}

func (f *rebaseFlow) start(root string, terminal ui.Terminal, rows []row, cursor int) tea.Cmd {
	next := cursor + 1
	for next < len(rows) && rows[next].kind != rowCommit {
		next++
	}
	if next >= len(rows) {
		return notify.Warning("rebase -i: no parent commit below selection")
	}

	hash := rows[next].commit.FullHash

	dirty, err := history.HasUnstagedChanges(root)
	if err != nil {
		return notify.Error("rebase -i: " + err.Error())
	}
	if dirty {
		f.confirm = rebaseConfirmState{kind: rebaseConfirmStash, yes: true, hash: hash}
		return nil
	}

	return f.cmdRun(root, terminal, hash)
}

func (f *rebaseFlow) handleConfirmUpdate(msg tea.Msg, root string) tea.Cmd {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	nextYes, decided, accepted, handled := components.UpdateConfirm(kp, f.confirm.yes)
	if !handled {
		return nil
	}
	if !decided {
		f.confirm.yes = nextYes
		return nil
	}
	kind := f.confirm.kind
	hash := f.confirm.hash
	f.confirm = rebaseConfirmState{}
	if !accepted {
		return nil
	}
	switch kind {
	case rebaseConfirmStash:
		return tea.Batch(
			notify.Progress("rebase-stash", "stashing..."),
			func() tea.Msg {
				_, err := history.StashPushAuto(root)
				return rebaseStashMsg{hash: hash, err: err}
			},
		)
	case rebaseConfirmStashPop:
		return tea.Batch(
			notify.Progress("rebase-stash-pop", "popping stash..."),
			func() tea.Msg {
				_, err := git.StashPop(root)
				return rebaseStashPopMsg{err: err}
			},
		)
	}
	return nil
}

func (f rebaseFlow) view(width int) string {
	var prompt string
	switch f.confirm.kind {
	case rebaseConfirmStash:
		prompt = "Stash unstaged changes before running git rebase -i?"
	case rebaseConfirmStashPop:
		prompt = "Rebase done. Pop the stash now?"
	}
	modalW := width / 2
	if modalW < 48 {
		modalW = 48
	}
	return components.RenderConfirmModal(prompt, f.confirm.yes, ui.ColorYellow, ui.ColorGreen, ui.ColorRed, ui.ColorSubtle, modalW)
}

func (f *rebaseFlow) handleStash(msg rebaseStashMsg, root string, terminal ui.Terminal) tea.Cmd {
	if msg.err != nil {
		return tea.Batch(notify.Close("rebase-stash"), notify.Error("stash failed: "+msg.err.Error()))
	}
	f.didStash = true
	return tea.Batch(notify.Close("rebase-stash"), f.cmdRun(root, terminal, msg.hash))
}

func (f rebaseFlow) cmdRun(root string, terminal ui.Terminal, hash string) tea.Cmd {
	return terminalrun.Command(root, terminal, "git", []string{"rebase", "-i", hash}, func(err error, splitApp string) tea.Msg {
		return rebaseFinishedMsg{err: err, splitApp: splitApp}
	})
}

// handleFinished takes the reload command from the caller because reloading
// the log needs Model state.
func (f *rebaseFlow) handleFinished(msg rebaseFinishedMsg, root string, reload tea.Cmd) tea.Cmd {
	if msg.err != nil && msg.splitApp == "" {
		return tea.Batch(notify.Error("rebase -i: "+msg.err.Error()), reload)
	}
	if f.didStash && msg.splitApp == "" {
		// exec mode: rebase done, pop stash now
		f.didStash = false
		return tea.Batch(
			notify.Progress("rebase-stash-pop", "popping stash..."),
			reload,
			nav.RepoMutated(),
			func() tea.Msg {
				_, err := git.StashPop(root)
				return rebaseStashPopMsg{err: err}
			},
		)
	}
	// kitty/tmux: didStash stays true; pop prompt fires on next FocusMsg.
	// Emit RepoMutated only on success; error in split-terminal mode may not have mutated.
	var mutatedCmd tea.Cmd
	if msg.err == nil {
		mutatedCmd = nav.RepoMutated()
	}
	return tea.Batch(mutatedCmd, reload)
}

func (f rebaseFlow) handleStashPop(msg rebaseStashPopMsg) tea.Cmd {
	if msg.err != nil {
		return tea.Batch(notify.Close("rebase-stash-pop"), notify.Error("stash pop failed: "+msg.err.Error()))
	}
	return notify.Close("rebase-stash-pop")
}
