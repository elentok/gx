package agents

import (
	"context"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

type sessionActionMsg struct {
	verb   string
	result server.SessionResult
	err    error
}

type sessionCall func(context.Context, string) (server.SessionResult, error)

// selected is the session under the cursor; ok is false when there is none.
func (m Model) selected() (server.SessionInfo, bool) {
	if m.client == nil || m.cursor >= len(m.sessions) {
		return server.SessionInfo{}, false
	}
	return m.sessions[m.cursor], true
}

// stopSelected asks before stopping: Stop always asks, even on a session
// already Exited, where the server's refusal explains.
func (m Model) stopSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:    "Stop session " + displayName(s) + "?",
		AcceptCmd: sessionActionCmd("stop", m.client.StopSession, s.ID),
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
	remove := sessionActionCmd("remove", m.client.RemoveSession, s.ID)
	if s.Status.State != server.SessionExited {
		return m, remove
	}
	m.confirm = m.confirm.Open(confirm.Options{Prompt: "Remove session " + displayName(s) + "?", AcceptCmd: remove})
	return m, nil
}

func (m Model) resumeSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	return m, sessionActionCmd("resume", m.client.ResumeSession, s.ID)
}

// actionSteps is what running a's Action does, one line per step: its prompts
// in order, or the queue it runs. Empty means there is no Action.
func actionSteps(a server.SessionActionable) []string {
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
		accept = queueActionCmd(m.client, s.Actionable.Target)
	} else {
		accept = sessionActionCmd("run action", m.client.RunSessionAction, s.ID)
	}
	m.confirm = m.confirm.Open(confirm.Options{
		Prompt:     "Run action on " + displayName(s) + "?",
		Items:      steps,
		AcceptCmd:  accept,
		DefaultYes: true,
	})
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
func queueActionCmd(c SessionClient, epic string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		snap, err := c.Snapshot(ctx)
		if err != nil {
			return queueActionMsg{err: err}
		}
		queued, refusals := ticketsui.QueueEpic(ctx, snap, c.QueueAdd, epic)
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
		return m, ticketsui.QueuedToast(msg.queued, msg.refusals)
	}
	return m, tea.Batch(ticketsui.QueuedToast(msg.queued, nil), nav.Switch(nav.ViewState{Tab: nav.TabQueue, InitialPath: msg.epic}))
}

func (m Model) markReadSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	return m, sessionActionCmd("mark read", m.client.MarkSessionRead, s.ID)
}

// sessionMenuPickMsg carries a menu pick back to Update: the session actions
// open the confirm on the model, which an item's Run cannot reach.
type sessionMenuPickMsg struct {
	id  string
	key string
}

// sessionMenuItems lists every session item in a fixed order; those that
// don't apply to s are dim with the reason.
func sessionMenuItems(s server.SessionInfo) []contextmenu.Item[server.SessionInfo] {
	live := s.Status.State != server.SessionExited
	item := func(label, key, pick, disabled string) contextmenu.Item[server.SessionInfo] {
		return contextmenu.Item[server.SessionInfo]{
			Label: label, Key: key, Group: contextmenu.Agent, Disabled: disabled,
			Run: func(s server.SessionInfo) tea.Cmd {
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
	items := []contextmenu.Item[server.SessionInfo]{
		item("Stop", "x", "x", reason(live, "exited")),
		item("Restore session", "r", "r", reason(!live, "running")),
		item("Remove", "d", "d", reason(!live, "running")),
		item("Mark read", "", "mark-read", reason(s.Unread.Unread, "nothing unread")),
		item("Run Action", "a", "a", reason(len(actionSteps(s.Actionable)) > 0, "no action")),
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
	m.menu = m.menu.Open(s, "Session "+displayName(s), sessionMenuItems(s))
	return m, nil
}

// menuAnchor is the selected session's sidebar row, under the panel's
// header line; the zero Anchor (a centered menu) while the sidebar is
// hidden.
func (m Model) menuAnchor() contextmenu.Anchor {
	if !m.sidebarOnScreen() {
		return contextmenu.Anchor{}
	}
	for i, r := range m.visibleRows() {
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
	case "auto-advance":
		s, _ := m.selected()
		return m, sessionActionCmd("auto-advance", func(ctx context.Context, id string) (server.SessionResult, error) {
			return m.client.SetAutoAdvance(ctx, id, !s.AutoAdvance)
		}, s.ID)
	case "cancel-auto-advance":
		return m, sessionActionCmd("cancel auto-advance", m.client.CancelAutoAdvance, msg.id)
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
	i := slices.IndexFunc(m.sessions, func(s server.SessionInfo) bool { return s.ID == resumed.ID })
	if i < 0 {
		return m, nil
	}
	m.sessions = slices.Clone(m.sessions)
	m.sessions[i] = resumed
	return m.selectAndFocus(i)
}
