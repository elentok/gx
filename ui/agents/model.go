package agents

import (
	"os"
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
	"github.com/elentok/gx/ui/clip"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/help"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/viewmodel"
)

var (
	keyDown          = key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "next session"))
	keyUp            = key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "previous session"))
	keyToggleSidebar = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "toggle sidebar"))
	keyFocusTerminal = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "focus terminal"))
	keyAttention     = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "next needing attention"))
	keyJump          = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "jump to session"))
	keyCreate        = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "new session"))
	keyStop          = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop session"))
	keyRemove        = key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "remove session"))
	keyResume        = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "resume session"))
	keyRunAction     = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "run action"))
	keyMenu          = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "session menu"))
)

type pageActivatedMsg struct{}

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
	afk         bool
	hideSidebar bool
	spinner     spinner.Model
	// ticking: a spinner tick chain is in flight, so another must not start.
	ticking bool
	// terminalFocused: the embedded terminal takes every key, so the shell
	// must not read gx tab keys.
	terminalFocused bool
	// leaderPending: the leader was pressed and the next key is its action.
	leaderPending bool
	// jump is nil while the jump popup is closed.
	jump *jumpPopup
	// create is nil while the create form is closed.
	create       *createForm
	listProjects func() ([]formProject, error)
	// cwdProject is the cwd repo's project; empty outside a registered one.
	cwdProject string
	// pendingSelect is a just-created or banner-clicked session to select and
	// focus once a snapshot lists it.
	pendingSelect string
	// confirm asks y/n before a Stop or Remove.
	confirm confirm.Model
	// menu is the `m` menu of the selected session's actions.
	menu          components.MenuState
	menuOpen      bool
	help          help.Model
	width, height int
	// sessionsDir holds the sessions' state dirs; empty when unknown.
	sessionsDir string
	lastScreen  lastScreen
	dial        attachDialer
	// attach is nil when no session is on screen.
	attach *attachment
	// copy is nil outside copy mode.
	copy      *copyMode
	clipWrite func(string) error
	now       func() time.Time
	// opened: the opening focus was picked, on the first snapshot.
	opened bool
	// lastViewed is the session viewed last by an earlier TUI.
	lastViewed string
	// focused: the TUI's terminal has focus.
	focused     bool
	termProgram string
	// reporter is nil without a client.
	reporter *focusReporter
}

// NewModel builds the tab; a nil client means there is no server to reach.
func NewModel(client SessionClient, settings ui.Settings) Model {
	sp := spinner.New()
	sp.Spinner = ui.ProgressSpinnerFor(settings.UseNerdFontIcons)
	m := Model{client: client, settings: settings, spinner: sp, dial: dialHolderAttach, clipWrite: clip.Write, now: time.Now, listProjects: listFormProjects,
		focused: true, termProgram: os.Getenv("TERM_PROGRAM")}
	if client != nil {
		m.reporter = &focusReporter{client: client}
	}
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
		m.afk = st.AFK
		selected := m.selectedID()
		m.sessions = orderedSessions(st.Sessions)
		m.cursor = 0
		for i, s := range m.sessions {
			if s.ID == selected {
				m.cursor = i
			}
		}
		if !m.opened {
			m.opened = true
			m = m.open()
		}
	}
	next, cmd := m.selectPending()
	next.(Model).reportFocus()
	return next, cmd
}

// InputFocused tells the shell the page swallows every key: in the terminal,
// after the leader, in the jump popup, create form, confirm or menu and while
// typing a help filter, and in copy mode.
func (m Model) InputFocused() bool {
	return m.terminalFocused || m.leaderPending || m.copy != nil || m.jump != nil || m.create != nil || m.confirm.IsOpen || m.menuOpen || m.help.InputFocused()
}

// TerminalFocused tells the shell ctrl+o is the page's: the agent's in the
// terminal, the leader's after it. Copy mode and popups leave it to the shell,
// so it opens the switcher or says to close the modal.
func (m Model) TerminalFocused() bool {
	return m.leaderPending || m.terminalFocused && m.copy == nil && !m.ModalOpen()
}

// ModalOpen is true while the jump, create, confirm, menu or help popup is open.
func (m Model) ModalOpen() bool {
	return m.jump != nil || m.create != nil || m.confirm.IsOpen || m.menuOpen || m.help.IsOpen
}

func (m Model) OnPageActivated() tea.Cmd {
	return func() tea.Msg { return pageActivatedMsg{} }
}

// OnPageDeactivated detaches, since only the session on screen is attached,
// and tells the server no session is on screen. The next activation redials.
func (m Model) OnPageDeactivated() tea.Cmd {
	m.attach.close()
	r := m.focusReport()
	r.Session = ""
	m.reporter.report(r)
	return nil
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	next.(Model).reportFocus()
	next.(Model).syncAttachSize()
	return next, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.FocusMsg:
		m.focused = true
		return m, nil
	case tea.BlurMsg:
		m.focused = false
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help, _ = m.help.Update(msg)
		return m.startSpinner()
	case lastScreenMsg:
		if msg.id == m.selectedID() {
			m.lastScreen = lastScreen(msg)
		}
		return m, nil
	case afkMsg:
		return m.updateAFK(msg)
	case attachedMsg:
		return m.updateAttached(msg)
	case attachRetryMsg:
		return m.updateAttachRetry(msg)
	case attachStreamMsg:
		return m.updateAttachStream(msg)
	case formProjectsMsg:
		return m.updateFormProjects(msg)
	case sessionCreatedMsg:
		return m.updateSessionCreated(msg)
	case sessionActionMsg:
		return m.updateSessionAction(msg)
	case pageActivatedMsg:
		// The spinner's tick chain died while the tab was hidden. Focus
		// reports reach only the page on screen, and switching tabs takes
		// the terminal's focus.
		m.ticking, m.focused = false, true
		return m.refresh()
	case tea.PasteMsg:
		if m.terminalFocused && m.copy == nil {
			return m, m.sendPaste(msg.Content)
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
		return m.updateKey(msg)
	case tea.MouseWheelMsg:
		return m.updateWheel(msg)
	case tea.MouseClickMsg:
		return m.updateClick(msg)
	}
	return m, nil
}

func (m Model) updateSidebarKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	case key.Matches(msg, keyAttention):
		m.cursor = m.nextNeedingAttention()
	case key.Matches(msg, keyJump):
		m.jump = &jumpPopup{}
		return m.startSpinner()
	case key.Matches(msg, keyCreate):
		return m.openCreate()
	case key.Matches(msg, keyStop):
		return m.stopSelected()
	case key.Matches(msg, keyRemove):
		return m.removeSelected()
	case key.Matches(msg, keyResume):
		return m.resumeSelected()
	case key.Matches(msg, keyRunAction):
		return m.runActionSelected()
	case key.Matches(msg, keyMenu):
		return m.openMenu()
	default:
		return m, nil
	}
	return m.refresh()
}

// selectAndFocus selects session i and focuses its terminal when it has one.
func (m Model) selectAndFocus(i int) (tea.Model, tea.Cmd) {
	m.cursor = i
	next, cmd := m.refresh()
	m = next.(Model)
	if m.attachTarget() != "" {
		m.terminalFocused = true
		m.resizeAttach()
	}
	return m, cmd
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
	var shown []int
	if m.jump != nil {
		shown = m.jumpShown()
	}
	if m.sidebarShown() {
		for _, r := range m.visibleRows() {
			if r.session >= 0 {
				shown = append(shown, r.session)
			}
		}
	}
	for _, i := range shown {
		if m.sessions[i].Status.State == server.SessionWorking {
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
	// The mouse mode also keeps the wheel from reaching the agent as arrow
	// keys, as a terminal sends it to a program in the alternate screen.
	v := ui.NewMainView(m.render())
	v.Cursor = m.terminalCursor()
	return v
}

func (m Model) render() string {
	lines := m.lines()
	if m.width <= 0 || m.height <= 0 {
		return strings.Join(lines, "\n")
	}
	var out string
	if m.sidebarOnScreen() {
		out = lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderSidebar(), ui.RenderSeamColumn(m.height, ui.SeamColor), m.renderMain(m.mainWidth(), lines))
	} else {
		out = m.renderMain(m.width, lines)
	}
	if m.jump != nil {
		out = ui.OverlayCenter(out, m.renderJump(), m.width, m.height)
	}
	if m.create != nil {
		out = ui.OverlayCenter(out, m.renderCreate(), m.width, m.height)
	}
	if m.menuOpen {
		out = ui.OverlayCenter(out, m.renderMenu(), m.width, m.height)
	}
	if m.confirm.IsOpen {
		out = ui.OverlayCenter(out, m.confirm.View(m.width), m.width, m.height)
	}
	if m.leaderPending {
		out = ui.OverlayBottomRight(out, ui.RenderChordOverlay(m.leaderKeys()[0], m.leaderBindings()), m.width, m.height)
	}
	if m.help.IsOpen {
		out = ui.OverlayCenter(out, m.help.View(), m.width, m.height)
	}
	return out
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
