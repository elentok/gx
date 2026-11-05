package modal

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

// PushDiverged asks how to resolve a push that the remote has diverged from.
type PushDiverged struct {
	divergence *git.PushDivergence
	wt         git.Worktree
	menu       components.MenuState
	// log is the last job output, handed on to the follow-up command.
	log string
}

// NewPushDiverged builds the menu for wt's divergence; log is the last job
// output, handed on to the follow-up command.
func NewPushDiverged(wt git.Worktree, div *git.PushDivergence, log string) PushDiverged {
	return PushDiverged{
		divergence: div,
		wt:         wt,
		log:        log,
		menu: components.MenuState{
			Items: []components.MenuItem{
				{Label: "Rebase", Value: "rebase"},
				{Label: "Push --force", Value: "force"},
				{Label: "Abort", Value: "abort"},
			},
			Cursor: 0,
		},
	}
}

func (p PushDiverged) Update(msg tea.Msg) (Modal, tea.Cmd, Result) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil, Result{Unhandled: true}
	}
	next, decided, accepted, handled := components.UpdateMenu(key, p.menu)
	if !handled {
		return p, nil, Result{}
	}
	p.menu = next
	if !decided {
		return p, nil, Result{}
	}
	if !accepted {
		return p, notify.Info(ui.MessageAborted("push")), Result{Close: true}
	}
	item, _ := p.menu.Selected()
	switch item.Value {
	case "rebase":
		return p, nil, Result{Close: true, Spinner: "Rebasing " + p.wt.Name + "...", Action: Rebase{Wt: p.wt, Upstream: p.divergence.Upstream, Log: p.log}}
	case "force":
		return p, nil, Result{Close: true, Action: ForcePush{Wt: p.wt, Log: p.log}}
	default:
		return p, notify.Info(ui.MessageAborted("push")), Result{Close: true}
	}
}

func (p PushDiverged) InputFocused() bool { return true }

func (p PushDiverged) View(_, _ int) string {
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
