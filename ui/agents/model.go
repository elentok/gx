package agents

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

// Model is the Agents tab: the server's interactive sessions.
type Model struct {
	// client is nil when no server client could be built.
	client   SessionClient
	link     ticketsui.ServerLink
	loaded   bool
	sessions []server.SessionInfo
	// terminalFocused: the embedded terminal takes every key, so the shell
	// must not read gx tab keys.
	terminalFocused bool
	width, height   int
}

// NewModel builds the tab; a nil client means there is no server to reach.
func NewModel(client SessionClient) Model {
	return Model{client: client}
}

// WithServerState hands the tab the app shell's shared state and the link it
// was read over; nil st means no snapshot has arrived yet.
func (m Model) WithServerState(st *viewmodel.State, link ticketsui.ServerLink) (tea.Model, tea.Cmd) {
	m.link = link
	if st != nil {
		m.loaded = true
		m.sessions = st.Sessions
	}
	return m, nil
}

// InputFocused tells the shell the page swallows every key.
func (m Model) InputFocused() bool {
	return m.terminalFocused
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = msg.Width, msg.Height
	}
	return m, nil
}

func (m Model) View() tea.View {
	lines := m.lines()
	if m.width <= 0 || m.height <= 0 {
		return tea.NewView(strings.Join(lines, "\n"))
	}
	opts := ui.PanelOptionsFor(m.width, m.height, "agents", "", lines, true, ui.ColorText, ui.ColorOrange, true)
	return tea.NewView(ui.RenderPanel(opts))
}

func (m Model) lines() []string {
	switch {
	case m.client == nil || m.link == ticketsui.ServerLinkDown:
		return []string{ui.StyleMuted.Render("no server running — agent sessions need the gx server")}
	case !m.loaded:
		return []string{ui.StyleMuted.Render("loading…")}
	case len(m.sessions) == 0:
		return []string{ui.StyleMuted.Render("c new session")}
	}
	lines := make([]string, len(m.sessions))
	for i, s := range m.sessions {
		lines[i] = displayName(s)
	}
	return lines
}

func displayName(s server.SessionInfo) string {
	if s.Name != "" {
		return s.Name
	}
	return s.ID
}
