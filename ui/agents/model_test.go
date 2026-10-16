package agents

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server"
	teatest "github.com/elentok/gx/testutil/teatestv2"
	"github.com/elentok/gx/ui"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

func startAgentsTUI(t *testing.T, m tea.Model) *teatest.TestModel {
	t.Helper()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 20))
	t.Cleanup(func() { _ = tm.Quit() })
	return tm
}

func waitForText(t *testing.T, tm *teatest.TestModel, text string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func([]byte) bool {
		return bytes.Contains([]byte(ansi.Strip(string(tm.CurrentFrame()))), []byte(text))
	}, teatest.WithDuration(3*time.Second))
}

func TestAgents_NoServerSaysAServerIsNeeded(t *testing.T) {
	for name, m := range map[string]tea.Model{
		"no client": NewModel(nil, ui.Settings{}),
		"link down": withState(t, NewModel(&fakeSessionClient{}, ui.Settings{}), nil, ticketsui.ServerLinkDown),
	} {
		t.Run(name, func(t *testing.T) {
			tm := startAgentsTUI(t, m)
			waitForText(t, tm, "agent sessions need the gx server")
		})
	}
}

func TestAgents_NoSessionsHintsNewSession(t *testing.T) {
	m := withState(t, NewModel(&fakeSessionClient{}, ui.Settings{}), &viewmodel.State{}, ticketsui.ServerLinkUp)
	tm := startAgentsTUI(t, m)
	waitForText(t, tm, "c new session")
}

func TestAgents_SessionsReplaceTheEmptyState(t *testing.T) {
	st := &viewmodel.State{Sessions: []server.SessionInfo{{ID: "s1", Name: "fix login"}}}
	m := withState(t, NewModel(&fakeSessionClient{}, ui.Settings{}), st, ticketsui.ServerLinkUp)
	tm := startAgentsTUI(t, m)
	waitForText(t, tm, "fix login")
	if bytes.Contains(tm.CurrentFrame(), []byte("c new session")) {
		t.Fatal("new-session hint shown with a session listed")
	}
}

func TestAgents_InputFocusedOnlyInTerminalFocus(t *testing.T) {
	m := NewModel(&fakeSessionClient{}, ui.Settings{})
	if m.InputFocused() {
		t.Fatal("sidebar focus must leave gx tab keys to the shell")
	}
	m.terminalFocused = true
	if !m.InputFocused() {
		t.Fatal("terminal focus must swallow every key")
	}
}

func withState(t *testing.T, m Model, st *viewmodel.State, link ticketsui.ServerLink) Model {
	t.Helper()
	next, _ := m.WithServerState(st, link)
	return next.(Model)
}
