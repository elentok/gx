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

// selected is the session under the cursor; ok is false when there is none.
func (m Model) selected() (wire.SessionInfo, bool) {
	if m.client == nil || m.cursor >= len(m.sessions) {
		return wire.SessionInfo{}, false
	}
	return m.sessions[m.cursor], true
}

// queueStopHint is shown instead of stop, remove and restore on a Queue agent.
const queueStopHint = "stop a Queue agent from the Queue tab"

// stopSelected asks before stopping: Stop always asks, even on a session
// already Exited, where the server's refusal explains.
func (m Model) stopSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	if s.Queue {
		return m, notify.Info(queueStopHint)
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    "Stop session " + agentstyle.DisplayName(s) + "?",
		AcceptCmd: sessionActionCmd("stop", m.sessionAPI.Stop, s.ID),
	})
	return m, nil
}

// removeSelected asks before removing an Exited session; a live one goes
// straight to the server, whose refusal says why it stays.
func (m Model) removeSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	if s.Queue {
		return m, notify.Info(queueStopHint)
	}
	remove := sessionActionCmd("remove", m.sessionAPI.Remove, s.ID)
	if s.Status.State != wire.SessionExited {
		return m, remove
	}
	m.confirm = m.confirm.Open(confirm.Options{Prompt: "Remove session " + agentstyle.DisplayName(s) + "?", AcceptCmd: remove})
	return m, nil
}

func (m Model) resumeSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	if s.Queue {
		return m, notify.Info(queueStopHint)
	}
	return m, sessionActionCmd("resume", m.sessionAPI.Resume, s.ID)
}

// actionSteps is what running a's Action does, one line per step: its prompts
// in order, or the queue it runs. Empty means there is no Action.
func actionSteps(a wire.SessionActionable) []string {
	if a.IsQueue() {
		return []string{"Queue `" + a.Target + "`"}
	}
	return a.Action
}

// runActionSelected asks before running the Action, listing every step;
// Enter confirms and the server refuses unless the session is Idle.
func (m Model) runActionSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	steps := actionSteps(s.Actionable)
	if len(steps) == 0 {
		return m, notify.Info("no action")
	}
	var accept tea.Cmd
	if s.Actionable.IsQueue() {
		accept = queueActionCmd(m.client, m.queue, s.Actionable.Target)
	} else {
		accept = sessionActionCmd("run action", m.sessionAPI.RunAction, s.ID)
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:     "Run action on " + agentstyle.DisplayName(s) + "?",
		Items:      steps,
		AcceptCmd:  accept,
		DefaultYes: true,
	})
	return m, nil
}

// unlockMsg lifts the read-only of a, once the person confirmed.
type unlockMsg struct{ a *terminal.Attachment }

// unlockSelected asks before letting keys and paste reach a Queue or Design
// agent; Cancel is the default.
func (m Model) unlockSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	a := m.attach
	if !ok || a == nil || a.ID != s.ID || !a.ReadOnly {
		return m, nil
	}
	prompt := "This Design agent is waiting on gx-design. Typing into it can derail the hand-off. Unlock input?"
	if s.Queue {
		prompt = "gx is driving this Queue agent. Typing into it can derail the queue. Unlock input?"
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    prompt,
		AcceptCmd: func() tea.Msg { return unlockMsg{a: a} },
	})
	return m, nil
}

func (m Model) updateUnlock(msg unlockMsg) (tea.Model, tea.Cmd) {
	if msg.a == m.attach && !msg.a.Closed {
		msg.a.ReadOnly = false
	}
	return m, nil
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

// updateQueueAction opens the Queue tab on the epic once the server took
// every ticket; a refusal, or nothing to queue, stays here as a warning.
func (m Model) updateQueueAction(msg queueActionMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		return m, notify.Error("queue failed: " + msg.err.Error())
	case msg.queued == 0 && len(msg.refusals) == 0:
		return m, notify.Warning("nothing to queue in " + msg.epic)
	case len(msg.refusals) > 0:
		return m, tabapi.QueuedToast(msg.queued, msg.refusals)
	}
	return m, tea.Batch(tabapi.QueuedToast(msg.queued, nil), nav.Switch(nav.ViewState{Tab: nav.TabQueue, InitialPath: msg.epic}))
}

func (m Model) markReadSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	return m, sessionActionCmd("mark read", m.sessionAPI.MarkRead, s.ID)
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

func (m Model) openMenu() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	m.menu = m.menu.Open(s, "Session "+agentstyle.DisplayName(s), sessionMenuItems(s))
	return m, nil
}

// menuAnchor is the selected session's sidebar row, under the panel's
// header line; the zero Anchor (a centered menu) while the sidebar is
// hidden.
func (m Model) menuAnchor() contextmenu.Anchor {
	if !m.sidebarOnScreen() {
		return contextmenu.Anchor{}
	}
	for i, r := range m.sidebar.visibleRows(m.sidebarInput()) {
		if r.session == m.cursor {
			return contextmenu.Anchor{X: 1, Y: 1 + i, H: 1}
		}
	}
	return contextmenu.Anchor{}
}

func (m Model) updateMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(msg)
	return m, cmd
}

// updateMenuPick runs a pick only while its session is still the selected
// one; the session actions work on the selection.
func (m Model) updateMenuPick(msg sessionMenuPickMsg) (tea.Model, tea.Cmd) {
	if msg.id != m.selectedID() {
		return m, nil
	}
	switch msg.key {
	case "x":
		return m.stopSelected()
	case "r":
		return m.resumeSelected()
	case "d":
		return m.removeSelected()
	case "mark-read":
		return m.markReadSelected()
	case "a":
		return m.runActionSelected()
	case "unlock":
		return m.unlockSelected()
	case "auto-advance":
		s, _ := m.selected()
		return m, sessionActionCmd("auto-advance", func(ctx context.Context, id string) (wire.SessionResult, error) {
			return m.sessionAPI.SetAutoAdvance(ctx, id, !s.AutoAdvance)
		}, s.ID)
	case "cancel-auto-advance":
		return m, sessionActionCmd("cancel auto-advance", m.sessionAPI.CancelAutoAdvance, msg.id)
	}
	return m, nil
}

func sessionActionCmd(verb string, call sessionCall, id string) tea.Cmd {
	return func() tea.Msg {
		res, err := call(context.Background(), id)
		return sessionActionMsg{verb: verb, result: res, err: err}
	}
}

func (m Model) updateConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.confirm, cmd, _ = m.confirm.Update(msg)
	return m, cmd
}

// updateSessionAction shows a failure as a footer message. A resumed session
// is shown live at once, without waiting for the next snapshot, so the panel
// attaches to its new holder.
func (m Model) updateSessionAction(msg sessionActionMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		return m, notify.Error(msg.verb + " failed: " + msg.err.Error())
	case msg.result.Refused:
		text := msg.result.Message
		if text == "" {
			text = msg.verb + " refused: " + msg.result.Reason
		}
		return m, notify.Error(text)
	case msg.verb != "resume" || msg.result.Session == nil:
		return m, nil
	}
	resumed := *msg.result.Session
	i := slices.IndexFunc(m.sessions, func(s wire.SessionInfo) bool { return s.ID == resumed.ID })
	if i < 0 {
		return m, nil
	}
	m.sessions = slices.Clone(m.sessions)
	m.sessions[i] = resumed
	return m.selectAndFocus(i)
}
