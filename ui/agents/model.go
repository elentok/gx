package agents

import (
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

var (
	keyDown          = key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "next session"))
	keyUp            = key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "previous session"))
	keyToggleSidebar = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "toggle sidebar"))
	keyFocusTerminal = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "focus terminal"))
)

// spinnerRestartMsg re-arms the Working spinner: its tick chain dies while
// the tab is hidden.
type spinnerRestartMsg struct{}

// Model is the Agents tab: the server's interactive sessions.
type Model struct {
	// client is nil when no server client could be built.
	client   SessionClient
	settings ui.Settings
	link     ticketsui.ServerLink
	loaded   bool
	// sessions is in sidebar order (see orderedSessions); cursor indexes it.
	sessions    []server.SessionInfo
	cursor      int
	hideSidebar bool
	spinner     spinner.Model
	// ticking: a spinner tick chain is in flight, so another must not start.
	ticking bool
	// terminalFocused: the embedded terminal takes every key, so the shell
	// must not read gx tab keys.
	terminalFocused bool
	width, height   int
	// sessionsDir holds the sessions' state dirs; empty when unknown.
	sessionsDir string
	lastScreen  lastScreen
	dial        attachDialer
	// attach is nil when no session is on screen.
	attach *attachment
	now    func() time.Time
}

// NewModel builds the tab; a nil client means there is no server to reach.
func NewModel(client SessionClient, settings ui.Settings) Model {
	sp := spinner.New()
	sp.Spinner = ui.ProgressSpinnerFor(settings.UseNerdFontIcons)
	m := Model{client: client, settings: settings, spinner: sp, dial: dialHolderAttach, now: time.Now}
	if data, err := config.DataDir(); err == nil {
		m.sessionsDir = filepath.Join(data, "sessions")
	}
	return m
}

// WithServerState hands the tab the app shell's shared state and the link it
// was read over; nil st means no snapshot has arrived yet.
func (m Model) WithServerState(st *viewmodel.State, link ticketsui.ServerLink) (tea.Model, tea.Cmd) {
	m.link = link
	if st != nil {
		m.loaded = true
		selected := m.selectedID()
		m.sessions = orderedSessions(st.Sessions)
		m.cursor = 0
		for i, s := range m.sessions {
			if s.ID == selected {
				m.cursor = i
			}
		}
	}
	return m.refresh()
}

// InputFocused tells the shell the page swallows every key.
func (m Model) InputFocused() bool {
	return m.terminalFocused
}

func (m Model) OnPageActivated() tea.Cmd {
	return func() tea.Msg { return spinnerRestartMsg{} }
}

// OnPageDeactivated detaches: only the session on screen is attached. The
// next activation redials.
func (m Model) OnPageDeactivated() tea.Cmd {
	m.attach.close()
	return nil
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.terminalFocused {
			m.resizeAttach()
		}
		return m.startSpinner()
	case lastScreenMsg:
		if msg.id == m.selectedID() {
			m.lastScreen = lastScreen(msg)
		}
		return m, nil
	case attachedMsg:
		return m.updateAttached(msg)
	case attachStreamMsg:
		return m.updateAttachStream(msg)
	case spinnerRestartMsg:
		m.ticking = false
		return m.refresh()
	case tea.PasteMsg:
		if m.terminalFocused {
			m.sendPaste(msg.Content)
		}
		return m, nil
	case spinner.TickMsg:
		if msg.ID != m.spinner.ID() {
			return m, nil
		}
		if !m.workingVisible() {
			m.ticking = false
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if m.terminalFocused {
			m.sendKey(msg)
			return m, nil
		}
		switch {
		case key.Matches(msg, keyFocusTerminal):
			if m.attachTarget() != "" {
				m.terminalFocused = true
				m.resizeAttach()
			}
			return m, nil
		case key.Matches(msg, keyDown):
			m.cursor = min(m.cursor+1, max(0, len(m.sessions)-1))
		case key.Matches(msg, keyUp):
			m.cursor = max(m.cursor-1, 0)
		case key.Matches(msg, keyToggleSidebar):
			m.hideSidebar = !m.hideSidebar
		default:
			return m, nil
		}
		return m.refresh()
	}
	return m, nil
}

// refresh starts what the newly shown sessions need: the spinner, the
// selected session's attach and the selected Exited session's last screen.
func (m Model) refresh() (tea.Model, tea.Cmd) {
	m, attach := m.syncAttach()
	next, spin := m.startSpinner()
	return next, tea.Batch(spin, attach, next.(Model).loadLastScreen())
}

// startSpinner starts the Working spinner's tick chain when a Working session
// is on screen; the chain stops itself once none is (idle cost).
func (m Model) startSpinner() (tea.Model, tea.Cmd) {
	if m.ticking || !m.workingVisible() {
		return m, nil
	}
	m.ticking = true
	return m, m.spinner.Tick
}

func (m Model) workingVisible() bool {
	if !m.sidebarShown() {
		return false
	}
	for _, r := range m.visibleRows() {
		if r.session >= 0 && m.sessions[r.session].Status.State == server.SessionWorking {
			return true
		}
	}
	return false
}

func (m Model) sidebarShown() bool {
	return !m.hideSidebar && m.loaded && m.client != nil && m.link != ticketsui.ServerLinkDown
}

func (m Model) selectedID() string {
	if m.cursor < len(m.sessions) {
		return m.sessions[m.cursor].ID
	}
	return ""
}

func (m Model) View() tea.View {
	lines := m.lines()
	if m.width <= 0 || m.height <= 0 {
		return tea.NewView(strings.Join(lines, "\n"))
	}
	if !m.sidebarOnScreen() {
		return tea.NewView(m.renderMain(m.width, lines))
	}
	return tea.NewView(lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderSidebar(), ui.RenderSeamColumn(m.height, ui.SeamColor), m.renderMain(m.mainWidth(), lines)))
}

func (m Model) sidebarOnScreen() bool {
	return m.sidebarShown() && m.width > sidebarWidth+1
}

func (m Model) mainWidth() int {
	if m.sidebarOnScreen() {
		return m.width - sidebarWidth - 1
	}
	return m.width
}

func (m Model) renderMain(width int, lines []string) string {
	active := m.terminalFocused || !m.sidebarShown()
	if lines != nil || m.selectedID() == "" {
		return ui.RenderPanel(ui.PanelOptionsFor(width, m.height, "agents", "", lines, active, ui.ColorText, ui.ColorOrange, false))
	}
	s := m.sessions[m.cursor]
	titleColor, headerBg := headerColors(active)
	title := ui.RenderRowWithBackground(m.terminalTitle(s, lipgloss.NewStyle().Foreground(titleColor).Bold(active)), headerBg)
	opts := ui.PanelOptionsFor(width, m.height, title, m.terminalRightTitle(s, width-2), m.terminalLines(s), active, titleColor, titleColor, false)
	return ui.RenderPanel(opts)
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
	return nil
}

func displayName(s server.SessionInfo) string {
	if s.Name != "" {
		return s.Name
	}
	return s.ID
}
