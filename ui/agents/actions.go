package agents

import (
	"context"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/notify"
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

// runActionSelected runs the Action at once: it only sends prompts the
// session's own agent wrote, and the server refuses unless it is Idle.
func (m Model) runActionSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	if len(s.Actionable.Action) == 0 {
		return m, notify.Info("no action")
	}
	return m, sessionActionCmd("run action", m.client.RunSessionAction, s.ID)
}

func (m Model) markReadSelected() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	return m, sessionActionCmd("mark read", m.client.MarkSessionRead, s.ID)
}

// sessionMenuItems lists the items that apply to s, in a fixed order; each
// Value is the action key it runs.
func sessionMenuItems(s server.SessionInfo) []components.MenuItem {
	live := s.Status.State != server.SessionExited
	var items []components.MenuItem
	add := func(applies bool, label, value string) {
		if applies {
			items = append(items, components.MenuItem{Label: label, Value: value})
		}
	}
	add(live, "Stop", "x")
	add(!live, "Resume", "r")
	add(!live, "Remove", "X")
	add(s.Unread.Unread, "Mark read", "mark-read")
	add(len(s.Actionable.Action) > 0, "Run Action", "a")
	return items
}

func (m Model) openMenu() (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		return m, nil
	}
	m.menu = components.MenuState{Items: sessionMenuItems(s)}
	m.menuOpen = true
	return m, nil
}

func (m Model) updateMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	next, decided, accepted, handled := components.UpdateMenu(msg, m.menu)
	if !handled {
		return m, nil
	}
	m.menu = next
	if !decided {
		return m, nil
	}
	m.menuOpen = false
	item, ok := m.menu.Selected()
	if !accepted || !ok {
		return m, nil
	}
	switch item.Value {
	case "x":
		return m.stopSelected()
	case "r":
		return m.resumeSelected()
	case "X":
		return m.removeSelected()
	case "mark-read":
		return m.markReadSelected()
	case "a":
		return m.runActionSelected()
	}
	return m, nil
}

func (m Model) renderMenu() string {
	prompt := "Session " + displayName(m.sessions[m.cursor]) + ":"
	return components.RenderMenuModal("Session", prompt, m.menu, "",
		ui.ColorBorder, ui.ColorBlue, ui.ColorSubtle, ui.ColorText, 48)
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
