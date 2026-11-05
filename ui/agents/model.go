package agents

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/agents/agentstyle"
	"github.com/elentok/gx/ui/agents/create"
	"github.com/elentok/gx/ui/agents/jump"
	"github.com/elentok/gx/ui/agents/terminal"
	"github.com/elentok/gx/ui/clip"
	"github.com/elentok/gx/ui/confirm"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/tickets/tabapi"
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
	keyRemove        = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remove session"))
	keyResume        = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restore session"))
	keyRunAction     = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "run action"))
	keyMenu          = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "session menu"))
	keyHelp          = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))
)

type pageActivatedMsg struct{}

// Model is the Agents tab: the server's interactive sessions.
type Model struct {
	// client is nil when no server client could be built.
	client SessionClient
	// sessionAPI is nil exactly when client is.
	sessionAPI Sessions
	// queue is nil exactly when client is.
	queue    Queue
	settings ui.Settings
	link     tabapi.ServerLink
	loaded   bool
	// sessions is in sidebar order (see orderedSessions); cursor indexes it.
	sessions    []wire.SessionInfo
	cursor      int
	afk         bool
	hideSidebar bool
	spinner     spinner.Model
	// joinMemo lets a frame that changes nothing visible skip the render and
	// its width measuring.
	joinMemo *ui.Memo
	sidebar  sidebar
	// ticking: a spinner tick chain is in flight, so another must not start.
	ticking bool
	// terminalFocused: the embedded terminal takes every key, so the shell
	// must not read gx tab keys.
	terminalFocused bool
	// leaderPending: the leader was pressed and the next key is its action.
	leaderPending bool
	// jump is nil while the jump popup is closed.
	jump *jump.Popup
	// create is nil while the create form is closed.
	create       *create.Form
	listProjects func() ([]create.Project, error)
	// cwdProject is the cwd repo's project; empty outside a registered one.
	cwdProject string
	// pendingSelect is a just-created or banner-clicked session to select and
	// focus once a snapshot lists it.
	pendingSelect string
	// confirm asks y/n before a Stop or Remove.
	confirm confirm.Model
	// menu is the `m` menu of the selected session's actions.
	menu          contextmenu.Menu[wire.SessionInfo]
	help          help.Model
	width, height int
	// term renders the terminal panel (see termView).
	term terminal.View
	dial terminal.AttachDialer
	// attach is nil when no session is on screen.
	attach *terminal.Attachment
	// copy is nil outside copy mode.
	copy      *terminal.CopyMode
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
func NewModel(client SessionClient, sessions Sessions, settings ui.Settings) Model {
	sp := spinner.New()
	sp.Spinner = ui.ProgressSpinnerFor(settings.UseNerdFontIcons)
	m := Model{client: client, sessionAPI: sessions, settings: settings, spinner: sp, sidebar: sidebar{memo: &ui.Memo{}}, joinMemo: &ui.Memo{}, term: terminal.View{MainMemo: &ui.Memo{}}, dial: terminal.DialHolderAttach, clipWrite: clip.Write, now: time.Now, listProjects: func() ([]create.Project, error) { return create.ListProjects(settings.TicketStore) },
		focused: true, termProgram: os.Getenv("TERM_PROGRAM")}
	if client != nil {
		m.reporter = &focusReporter{sessions: sessions}
	}
	if data, err := settings.Dirs.DataDir(); err == nil {
		m.term.SessionsDir = filepath.Join(data, "sessions")
	}
	m.sidebar.groupsPath = groupsFile(settings.Dirs)
	m.sidebar.showQueue, m.sidebar.showDesign = loadGroups(m.sidebar.groupsPath)
	return m
}

// WithQueue gives the tab the queue sub-client.
func (m Model) WithQueue(q Queue) Model {
	m.queue = q
	return m
}

// WithServerState hands the tab the app shell's shared state and the link it
// was read over; nil st means no snapshot has arrived yet.
func (m Model) WithServerState(st *viewmodel.State, link tabapi.ServerLink) (tea.Model, tea.Cmd) {
	m.link = link
	if st != nil {
		m.loaded = true
		m.afk = st.AFK
		sessions := orderedSessions(st.Sessions)
		m.cursor = followSelection(m.sessions, m.cursor, sessions)
		m.sessions = sessions
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
	return m.terminalFocused || m.leaderPending || m.sidebar.togglePending || m.copy != nil || m.jump != nil || m.create != nil || m.confirm.IsOpen || m.menu.IsOpen || m.help.InputFocused()
}

// TerminalFocused tells the shell ctrl+o is the page's: the agent's in the
// terminal, the leader's after it. Copy mode and popups leave it to the shell,
// so it opens the switcher or says to close the modal.
func (m Model) TerminalFocused() bool {
	return m.leaderPending || m.terminalFocused && m.copy == nil && !m.ModalOpen()
}

// ModalOpen is true while the jump, create, confirm, menu or help popup is open.
func (m Model) ModalOpen() bool {
	return m.jump != nil || m.create != nil || m.confirm.IsOpen || m.menu.IsOpen || m.help.IsOpen
}

func (m Model) OnPageActivated() tea.Cmd {
	return func() tea.Msg { return pageActivatedMsg{} }
}

// OnPageDeactivated detaches, since only the session on screen is attached,
// and tells the server no session is on screen. The next activation redials.
func (m Model) OnPageDeactivated() tea.Cmd {
	m.attach.Close()
	r := m.focusReport()
	r.Session = ""
	m.reporter.report(r)
	return nil
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	nm := next.(Model)
	nm.sidebar.refreshHover(nm.ModalOpen())
	nm.reportFocus()
	if nm.terminalFocused {
		nm.attach.SyncSize(nm.termView().ScreenSize())
	}
	return nm, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if nav.IsOpenHelp(msg) {
		// Copy mode blocks it like a modal: it takes every key, so its own
		// leader can't reach "?" either.
		if m.ModalOpen() || m.copy != nil {
			return m, nil
		}
		m.leaderPending = false
		return m.openHelp(), nil
	}
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
	case terminal.LastScreenMsg:
		if msg.ID == m.selectedID() {
			m.term.LastScreen = terminal.LastScreen(msg)
		}
		return m, nil
	case afkMsg:
		return m.updateAFK(msg)
	case terminal.AttachedMsg:
		return m, m.attach.UpdateAttached(msg)
	case terminal.AttachRetryMsg:
		if !m.attach.Current(msg.A) {
			return m, nil
		}
		return m.dialAttach(msg.A.Attempt + 1)
	case terminal.AttachStreamMsg:
		res, cmd := m.attach.UpdateStream(msg)
		if res.Exited {
			m.terminalFocused = false
			// Shown until the server reports the exit and the saved last screen loads.
			m.term.LastScreen = terminal.LastScreen{ID: m.attach.ID, Rows: m.attach.Rows}
			m.copy = nil
		}
		if res.Copied != nil {
			cmd = tea.Batch(terminal.WriteCopied(m.clipWrite, *res.Copied), cmd)
		}
		return m, cmd
	case create.ProjectsMsg:
		if m.create != nil {
			m.create = m.create.UpdateProjects(msg, m.createCtx())
		}
		return m, nil
	case create.CreatedMsg:
		if m.create == nil {
			return m, nil
		}
		var res create.Result
		m.create, res = m.create.UpdateCreated(msg)
		if res.CreatedID == "" {
			return m, nil
		}
		m.pendingSelect = res.CreatedID
		return m.selectPending()
	case sessionActionMsg:
		return m.updateSessionAction(msg)
	case queueActionMsg:
		return m.updateQueueAction(msg)
	case sessionMenuPickMsg:
		return m.updateMenuPick(msg)
	case unlockMsg:
		return m.updateUnlock(msg)
	case pageActivatedMsg:
		// The spinner's tick chain died while the tab was hidden. Focus
		// reports reach only the page on screen, and switching tabs takes
		// the terminal's focus.
		m.ticking, m.focused = false, true
		return m.refresh()
	case tea.PasteMsg:
		if m.terminalFocused && m.copy == nil {
			return m, m.attach.SendPaste(msg.Content)
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
		v := m.termView()
		if dir, ok := ui.WheelDirection(msg); ok && v.OverPanel(msg.Mouse()) {
			x, y, inCell := v.ScreenCell(msg.Mouse())
			m.attach.Wheel(dir, x, y, inCell)
		}
		return m, nil
	case tea.MouseClickMsg:
		return m.updateClick(msg)
	case tea.MouseReleaseMsg:
		m.attach.ForwardMove(msg.Mouse(), screen.MouseRelease, m.termView().ClampedCell)
		return m, nil
	case tea.MouseMotionMsg:
		m.sidebar.hoverAt = msg.Mouse()
		m.attach.ForwardMove(msg.Mouse(), screen.MouseDrag, m.termView().ClampedCell)
		return m, nil
	}
	return m, nil
}

func (m Model) updateSidebarKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.sidebar.togglePending {
		m.sidebar.togglePending = false
		m.sidebar = m.sidebar.toggleGroup(msg.String())
		return m.refresh()
	}
	switch {
	case key.Matches(msg, keyFocusTerminal):
		m.focusTerminal()
		return m, nil
	case msg.String() == "t":
		m.sidebar.togglePending = true
		return m, nil
	case key.Matches(msg, keyDown):
		m.cursor = m.sidebar.stepCursor(m.sidebarInput(), 1)
	case key.Matches(msg, keyUp):
		m.cursor = m.sidebar.stepCursor(m.sidebarInput(), -1)
	case key.Matches(msg, keyToggleSidebar):
		m.hideSidebar = !m.hideSidebar
	case key.Matches(msg, keyAttention):
		m.cursor = m.nextNeedingAttention()
	case key.Matches(msg, keyJump):
		m.jump = &jump.Popup{}
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
	case key.Matches(msg, keyHelp):
		return m.openHelp(), nil
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
	m.focusTerminal()
	return m, cmd
}

// focusTerminal focuses the selected session's terminal when it has one.
func (m *Model) focusTerminal() {
	if m.attachTarget() != "" {
		m.terminalFocused = true
		m.attach.Resize(m.termView().ScreenSize())
	}
}

// refresh starts what the newly shown sessions need: the spinner, the
// selected session's attach and the selected Exited session's last screen.
func (m Model) refresh() (tea.Model, tea.Cmd) {
	m, attach := m.syncAttach()
	next, spin := m.startSpinner()
	return next, tea.Batch(spin, attach, next.(Model).termView().LoadLastScreen())
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
		shown = m.jump.Shown(m.jumpCtx())
	}
	if m.sidebarShown() {
		for _, r := range m.sidebar.visibleRows(m.sidebarInput()) {
			if r.session >= 0 {
				shown = append(shown, r.session)
			}
		}
	}
	for _, i := range shown {
		if m.sessions[i].Status.State == wire.SessionWorking {
			return true
		}
	}
	return false
}

func (m Model) sidebarShown() bool {
	return !m.hideSidebar && m.loaded && m.client != nil && m.link != tabapi.ServerLinkDown
}

// followSelection is the cursor into next that keeps the session selected in
// prev. When that session is gone it moves to the nearest one above it that
// remains, so removing a session selects the one above; else the first.
func followSelection(prev []wire.SessionInfo, cursor int, next []wire.SessionInfo) int {
	index := func(id string) int {
		return slices.IndexFunc(next, func(s wire.SessionInfo) bool { return s.ID == id })
	}
	for i := min(cursor, len(prev)-1); i >= 0; i-- {
		if j := index(prev[i].ID); j >= 0 {
			return j
		}
	}
	return 0
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
	// All-motion is safe with an agent that reports the mouse: gx forwards it
	// only the wheel and the buttons it holds (see updateWheel, forwardPress),
	// and the mode drops the motion it did not ask for.
	v := ui.NewMainView(m.render())
	v.MouseMode = tea.MouseModeAllMotion
	v.Cursor = m.termView().TerminalCursor()
	return v
}

// termView is the terminal panel's view over the tab's current state.
func (m Model) termView() terminal.View {
	in := terminal.Input{
		Settings: m.settings, Spinner: m.spinner.View(), AFK: m.afk, Now: m.now,
		Width: m.width, Height: m.height, MainWidth: m.mainWidth(),
		Sessions: m.sessions, Cursor: m.cursor, Attach: m.attach, Copy: m.copy,
		Focused: m.terminalFocused, Covered: m.ModalOpen() || m.leaderPending, Selected: m.selectedID(),
		DisplayName: agentstyle.DisplayName,
	}
	v := m.term.WithInput(in)
	if m.copy != nil {
		c, a := m.copy, m.attach
		in.CopyRows = func(rows []string) []string {
			cols, h := v.ScreenSize()
			return c.CopyRows(rows, terminal.NewAttachScreen(a, cols, h).Screen())
		}
		v = m.term.WithInput(in)
	}
	return v
}

func (m Model) render() string {
	lines := m.lines()
	if m.width <= 0 || m.height <= 0 {
		return strings.Join(lines, "\n")
	}
	var out string
	if m.sidebarOnScreen() {
		sidebar, main := m.sidebar.render(m.sidebarInput()), m.renderMain(m.mainWidth(), lines)
		out = m.joinMemo.Get([3]any{sidebar, main, m.height}, func() string {
			return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, ui.RenderSeamColumn(m.height, ui.SeamColor), main)
		})
	} else {
		out = m.renderMain(m.width, lines)
	}
	if m.jump != nil {
		out = ui.OverlayCenter(out, m.jump.Render(m.jumpCtx()), m.width, m.height)
	}
	if m.create != nil {
		out = ui.OverlayCenter(out, m.create.View(m.width, m.height, m.sessions), m.width, m.height)
	}
	if m.menu.IsOpen {
		out = m.menu.Overlay(out, m.menuAnchor(), m.width, m.height)
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
		opts := ui.PanelOptionsFor(width, m.height, "agents", "", lines, active, ui.ColorText, ui.ColorOrange, false)
		return m.term.MainMemo.Get(opts, func() string { return ui.RenderPanel(opts) })
	}
	s, v := m.sessions[m.cursor], m.termView()
	titleColor, headerBg := headerColors(active)
	title := ui.RenderRowWithBackground(v.TerminalTitle(s, lipgloss.NewStyle().Foreground(titleColor).Bold(active)), headerBg)
	opts := ui.PanelOptionsFor(width, m.height, title, v.TerminalRightTitle(s, width-2), v.TerminalLines(s), active, titleColor, titleColor, false)
	return m.term.MainMemo.Get(opts, func() string { return ui.RenderPanel(opts) })
}

func (m Model) lines() []string {
	switch {
	case m.client == nil || m.link == tabapi.ServerLinkDown:
		return []string{ui.StyleMuted.Render("no server running — agent sessions need the gx server")}
	case !m.loaded:
		return []string{ui.StyleMuted.Render("loading…")}
	case len(m.sessions) == 0:
		return []string{ui.StyleMuted.Render("c new session")}
	}
	return nil
}
