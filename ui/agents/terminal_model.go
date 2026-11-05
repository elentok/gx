package agents

import (
	"cmp"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/agents/terminal"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// attachTarget is the session the panel shows live, or "" for none.
func (m Model) attachTarget() string {
	if m.client == nil || m.link == tabapi.ServerLinkDown || !m.loaded || m.term.SessionsDir == "" || m.cursor >= len(m.sessions) {
		return ""
	}
	if s := m.sessions[m.cursor]; s.Status.State != wire.SessionExited {
		return s.ID
	}
	return ""
}

// syncAttach keeps exactly the on-screen session attached: it closes the
// attach of a session no longer shown and dials the one now shown.
func (m Model) syncAttach() (Model, tea.Cmd) {
	want := m.attachTarget()
	if a := m.attach; a != nil && !a.Closed && a.ID == want && !a.RedialDue(m.sessions[m.cursor]) {
		return m, nil
	}
	m.attach.Close()
	m.attach = nil
	if want == "" {
		return m, nil
	}
	return m.dialAttach(0)
}

// dialAttach replaces the attach with a new dial of the selected session.
func (m Model) dialAttach(attempt int) (Model, tea.Cmd) {
	m.attach.Close()
	s := m.sessions[m.cursor]
	a := &terminal.Attachment{ID: s.ID, Status: s.Status, Attempt: attempt, ReadOnly: s.Queue || s.Design}
	m.attach = a
	// A Queue agent's holder lives under the agents root, not the sessions dir.
	dir, dial := cmp.Or(s.Holder, filepath.Join(m.term.SessionsDir, s.ID)), m.dial
	return m, func() tea.Msg {
		stream, snapshot, err := dial(dir)
		return terminal.AttachedMsg{A: a, Stream: stream, Snapshot: snapshot, Err: err}
	}
}

// enterCopyMode starts copy mode on the bottom-most non-blank row in view.
func (m Model) enterCopyMode() (tea.Model, tea.Cmd) {
	a := m.attach
	if a == nil || a.Stream == nil || a.Ended {
		return m, nil
	}
	m.copy = terminal.NewCopyMode(a, m.copyScreen().Screen())
	return m, nil
}

// copyScreen is the attach at the panel's current size.
func (m Model) copyScreen() terminal.AttachScreen {
	cols, rows := m.termView().ScreenSize()
	return terminal.NewAttachScreen(m.attach, cols, rows)
}

func (m Model) updateCopyKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	c := m.copy
	if c.A != m.attach || c.A.Ended {
		m.copy = nil
		return m, nil
	}
	c, cmd, res := c.Update(msg, m.copyScreen())
	if res.Done {
		c = nil
	}
	m.copy = c
	return m, cmd
}
