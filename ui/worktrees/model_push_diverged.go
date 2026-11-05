package worktrees

import (
	"fmt"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
	humanize "github.com/dustin/go-humanize"
)

// pushDivergedModal asks how to resolve a push that the remote has diverged from.
type pushDivergedModal struct {
	divergence *git.PushDivergence
	wt         git.Worktree
	menu       components.MenuState
	// log is the last job output, handed on to the follow-up command.
	log string
}

func (m Model) enterPushDivergedMode(wt git.Worktree, div *git.PushDivergence) Model {
	return m.openModal(pushDivergedModal{
		divergence: div,
		wt:         wt,
		log:        m.lastJobLog,
		menu: components.MenuState{
			Items: []components.MenuItem{
				{Label: "Rebase", Value: "rebase"},
				{Label: "Push --force", Value: "force"},
				{Label: "Abort", Value: "abort"},
			},
			Cursor: 0,
		},
	})
}

func (p pushDivergedModal) Update(msg tea.Msg) (modal, tea.Cmd, modalResult) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil, modalResult{unhandled: true}
	}
	next, decided, accepted, handled := components.UpdateMenu(key, p.menu)
	if !handled {
		return p, nil, modalResult{}
	}
	p.menu = next
	if !decided {
		return p, nil, modalResult{}
	}
	if !accepted {
		return p, notify.Info(ui.MessageAborted("push")), modalResult{close: true}
	}
	item, _ := p.menu.Selected()
	switch item.Value {
	case "rebase":
		return p, cmdRebaseRef(p.wt, p.divergence.Upstream, p.log), modalResult{close: true, spinner: "Rebasing " + p.wt.Name + "..."}
	case "force":
		return p, cmdStartPromptableJob(promptableJobForcePush, p.wt, p.log, false), modalResult{close: true}
	default:
		return p, notify.Info(ui.MessageAborted("push")), modalResult{close: true}
	}
}

func (p pushDivergedModal) InputFocused() bool { return true }

func (p pushDivergedModal) View(_, _ int) string {
	d := p.divergence
	prompt := fmt.Sprintf(
		"Branch %s has diverged from the remote branch:\n\nLast local commit: %s\n  %s %s\n\nLast remote commit: %s\n  %s %s",
		d.Branch,
		humanizeOrUnknownTime(d.Local.Date),
		d.Local.Hash,
		d.Local.Message,
		humanizeOrUnknownTime(d.RemoteHead.Date),
		d.RemoteHead.Hash,
		d.RemoteHead.Message,
	)
	return components.RenderMenuModal(
		"Push Diverged",
		prompt,
		p.menu,
		"",
		ui.ColorBorder,
		ui.ColorBorder,
		ui.ColorGray,
		ui.ColorGreen,
		0,
	)
}

func humanizeOrUnknownTime(t time.Time) string {
	if t.IsZero() {
		return "unknown time"
	}
	return humanize.Time(t)
}
