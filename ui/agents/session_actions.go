package agents

import (
	"context"
	"slices"

	"github.com/elentok/gx/server/wire"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/agents/agentstyle"
	"github.com/elentok/gx/ui/agents/terminal"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

type sessionActionMsg struct {
	verb   string
	result wire.SessionResult
	err    error
}

type sessionCall func(context.Context, string) (wire.SessionResult, error)

// sessionActions is what the person can do to the selected session: the
// server calls, the y/n confirm in front of them and the `m` menu.
type sessionActions struct {
	// client is nil when no server client could be built.
	client SessionClient
	// sessionAPI is nil exactly when client is.
	sessionAPI Sessions
	// queue is nil exactly when client is.
	queue Queue
	// confirm asks y/n before a Stop or Remove.
	confirm confirm.Model
	// menu is the `m` menu of the selected session's actions.
	menu contextmenu.Menu[wire.SessionInfo]
}

// queueStopHint is shown instead of stop, remove and restore on a Queue agent.
const queueStopHint = "stop a Queue agent from the Queue tab"

// pick runs the action named by key on s, the selected session; ok is false
// when there is none. The sidebar keys x, d, r and a are the menu's pick keys
// of the same actions. attach is the terminal on screen, which unlock needs.
func (a *sessionActions) pick(key string, s wire.SessionInfo, ok bool, attach *terminal.Attachment) tea.Cmd {
	if !ok {
		return nil
	}
	switch key {
	case "x":
		return a.stop(s)
	case "r":
		return a.resume(s)
	case "d":
		return a.remove(s)
	case "mark-read":
		return sessionActionCmd("mark read", a.sessionAPI.MarkRead, s.ID)
	case "a":
		return a.runAction(s)
	case "unlock":
		return a.unlock(s, attach)
	case "auto-advance":
		return sessionActionCmd("auto-advance", func(ctx context.Context, id string) (wire.SessionResult, error) {
			return a.sessionAPI.SetAutoAdvance(ctx, id, !s.AutoAdvance)
		}, s.ID)
	case "cancel-auto-advance":
		return sessionActionCmd("cancel auto-advance", a.sessionAPI.CancelAutoAdvance, s.ID)
	}
	return nil
}

// stop asks before stopping: Stop always asks, even on a session already
// Exited, where the server's refusal explains.
func (a *sessionActions) stop(s wire.SessionInfo) tea.Cmd {
	if s.Queue {
		return notify.Info(queueStopHint)
	}
	a.confirm = a.confirm.Open(confirm.Options{
		Prompt:    "Stop session " + agentstyle.DisplayName(s) + "?",
		AcceptCmd: sessionActionCmd("stop", a.sessionAPI.Stop, s.ID),
	})
	return nil
}

// remove asks before removing an Exited session; a live one goes straight to
// the server, whose refusal says why it stays.
func (a *sessionActions) remove(s wire.SessionInfo) tea.Cmd {
	if s.Queue {
		return notify.Info(queueStopHint)
	}
	remove := sessionActionCmd("remove", a.sessionAPI.Remove, s.ID)
	if s.Status.State != wire.SessionExited {
		return remove
	}
	a.confirm = a.confirm.Open(confirm.Options{Prompt: "Remove session " + agentstyle.DisplayName(s) + "?", AcceptCmd: remove})
	return nil
}

func (a *sessionActions) resume(s wire.SessionInfo) tea.Cmd {
	if s.Queue {
		return notify.Info(queueStopHint)
	}
	return sessionActionCmd("resume", a.sessionAPI.Resume, s.ID)
}

// actionSteps is what running a's Action does, one line per step: its prompts
// in order, or the queue it runs. Empty means there is no Action.
func actionSteps(a wire.SessionActionable) []string {
	if a.IsQueue() {
		return []string{"Queue `" + a.Target + "`"}
	}
	return a.Action
}

// runAction asks before running the Action, listing every step; Enter
// confirms and the server refuses unless the session is Idle.
func (a *sessionActions) runAction(s wire.SessionInfo) tea.Cmd {
	steps := actionSteps(s.Actionable)
	if len(steps) == 0 {
		return notify.Info("no action")
	}
	var accept tea.Cmd
	if s.Actionable.IsQueue() {
		accept = queueActionCmd(a.client, a.queue, s.Actionable.Target)
	} else {
		accept = sessionActionCmd("run action", a.sessionAPI.RunAction, s.ID)
	}
	a.confirm = a.confirm.Open(confirm.Options{
		Prompt:     "Run action on " + agentstyle.DisplayName(s) + "?",
		Items:      steps,
		AcceptCmd:  accept,
		DefaultYes: true,
	})
	return nil
}

// unlockMsg lifts the read-only of a, once the person confirmed.
type unlockMsg struct{ a *terminal.Attachment }

// unlock asks before letting keys and paste reach a Queue or Design agent;
// Cancel is the default.
func (a *sessionActions) unlock(s wire.SessionInfo, att *terminal.Attachment) tea.Cmd {
	if att == nil || att.ID != s.ID || !att.ReadOnly {
		return nil
	}
	prompt := "This Design agent is waiting on gx-design. Typing into it can derail the hand-off. Unlock input?"
	if s.Queue {
		prompt = "gx is driving this Queue agent. Typing into it can derail the queue. Unlock input?"
	}
	a.confirm = a.confirm.Open(confirm.Options{
		Prompt:    prompt,
		AcceptCmd: func() tea.Msg { return unlockMsg{a: att} },
	})
	return nil
}

// queueActionMsg is the outcome of a queue action: how many tickets of epic
// the server took and the refusals as "address: reason".
type queueActionMsg struct {
	epic     string
	queued   int
	refusals []string
	err      error
}

// queueActionCmd queues the open tickets of epic ("project:epic"). The queue
// action never goes to the session as a prompt.
func queueActionCmd(c SessionClient, q Queue, epic string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		snap, err := c.Snapshot(ctx)
		if err != nil {
			return queueActionMsg{err: err}
		}
		queued, refusals := tabapi.QueueEpic(ctx, snap, q.Add, epic)
		return queueActionMsg{epic: epic, queued: queued, refusals: refusals}
	}
}

// queueActionResult opens the Queue tab on the epic once the server took
// every ticket; a refusal, or nothing to queue, stays here as a warning.
func queueActionResult(msg queueActionMsg) tea.Cmd {
	switch {
	case msg.err != nil:
		return notify.Error("queue failed: " + msg.err.Error())
	case msg.queued == 0 && len(msg.refusals) == 0:
		return notify.Warning("nothing to queue in " + msg.epic)
	case len(msg.refusals) > 0:
		return tabapi.QueuedToast(msg.queued, msg.refusals)
	}
	return tea.Batch(tabapi.QueuedToast(msg.queued, nil), nav.Switch(nav.ViewState{Tab: nav.TabQueue, InitialPath: msg.epic}))
}

// sessionMenuPickMsg carries a menu pick back to Update: the session actions
// open the confirm on the model, which an item's Run cannot reach.
type sessionMenuPickMsg struct {
	id  string
	key string
}

// sessionMenuItems lists every session item in a fixed order; those that
// don't apply to s are dim with the reason.
func sessionMenuItems(s wire.SessionInfo) []contextmenu.Item[wire.SessionInfo] {
	live := s.Status.State != wire.SessionExited
	item := func(label, key, pick, disabled string) contextmenu.Item[wire.SessionInfo] {
		return contextmenu.Item[wire.SessionInfo]{
			Label: label, Key: key, Group: contextmenu.Agent, Disabled: disabled,
			Run: func(s wire.SessionInfo) tea.Cmd {
				return func() tea.Msg { return sessionMenuPickMsg{id: s.ID, key: pick} }
			},
		}
	}
	reason := func(applies bool, why string) string {
		if applies {
			return ""
		}
		return why
	}
	var items []contextmenu.Item[wire.SessionInfo]
	if !s.Queue {
		items = append(items,
			item("Stop", "x", "x", reason(live, "exited")),
			item("Restore session", "r", "r", reason(!live, "running")),
			item("Remove", "d", "d", reason(!live, "running")),
		)
	}
	items = append(items,
		item("Mark read", "", "mark-read", reason(s.Unread.Unread, "nothing unread")),
		item("Run Action", "a", "a", reason(len(actionSteps(s.Actionable)) > 0, "no action")),
	)
	if s.Queue || s.Design {
		items = append(items, item("Unlock input", "i", "unlock", reason(live, "exited")))
	}
	if s.Design {
		label := "Auto-advance: off"
		if s.AutoAdvance {
			label = "Auto-advance: on"
		}
		items = append(items, item(label, "", "auto-advance", ""))
		if s.AutoAdvanceAt != nil {
			items = append(items, item("Cancel auto-advance", "", "cancel-auto-advance", ""))
		}
	}
	return items
}

func (a *sessionActions) openMenu(s wire.SessionInfo, ok bool) {
	if ok {
		a.menu = a.menu.Open(s, "Session "+agentstyle.DisplayName(s), sessionMenuItems(s))
	}
}

func (a *sessionActions) updateMenuKey(msg tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	a.menu, cmd = a.menu.Update(msg)
	return cmd
}

func (a *sessionActions) updateConfirmKey(msg tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	a.confirm, cmd, _ = a.confirm.Update(msg)
	return cmd
}

func sessionActionCmd(verb string, call sessionCall, id string) tea.Cmd {
	return func() tea.Msg {
		res, err := call(context.Background(), id)
		return sessionActionMsg{verb: verb, result: res, err: err}
	}
}

// sessionActionResult is what a finished session action asks of the model:
// a command to run, or a resumed session to put at Index in the list.
type sessionActionResult struct {
	Cmd     tea.Cmd
	Resumed *wire.SessionInfo
	Index   int
}

// updateSessionAction shows a failure as a footer message. A resumed session
// is shown live at once, without waiting for the next snapshot, so the panel
// attaches to its new holder.
func updateSessionAction(msg sessionActionMsg, sessions []wire.SessionInfo) sessionActionResult {
	switch {
	case msg.err != nil:
		return sessionActionResult{Cmd: notify.Error(msg.verb + " failed: " + msg.err.Error())}
	case msg.result.Refused:
		text := msg.result.Message
		if text == "" {
			text = msg.verb + " refused: " + msg.result.Reason
		}
		return sessionActionResult{Cmd: notify.Error(text)}
	case msg.verb != "resume" || msg.result.Session == nil:
		return sessionActionResult{}
	}
	resumed := *msg.result.Session
	i := slices.IndexFunc(sessions, func(s wire.SessionInfo) bool { return s.ID == resumed.ID })
	if i < 0 {
		return sessionActionResult{}
	}
	return sessionActionResult{Resumed: &resumed, Index: i}
}
