package app

import (
	"strings"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	agentsui "github.com/elentok/gx/ui/agents"
	commitui "github.com/elentok/gx/ui/commit"
	"github.com/elentok/gx/ui/keys"
	logui "github.com/elentok/gx/ui/log"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/navstate"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/notifyhistory"
	"github.com/elentok/gx/ui/notifylog"
	prsui "github.com/elentok/gx/ui/prs"
	"github.com/elentok/gx/ui/reloadgate"
	stashlistui "github.com/elentok/gx/ui/stashlist"
	statusui "github.com/elentok/gx/ui/status"
	ticketsui "github.com/elentok/gx/ui/tickets"
	"github.com/elentok/gx/ui/worktrees"
)

type Settings struct {
	InitialRoute       nav.ViewState
	ActiveWorktreePath string
	Server             *ServerDeps
	ui.Settings
}

type historyEntry struct {
	viewState nav.ViewState
	model     tea.Model
}

type livePage struct {
	model     tea.Model
	didInit   bool
	viewState nav.ViewState // the ViewState when this page was last created or re-routed
}

type Model struct {
	repo     git.Repo
	settings Settings

	width  int
	height int

	navState      navstate.State
	livePageByTab map[nav.TabID]livePage
	// history is the model side of the global deep-navigation stack.
	// navState.State holds the parallel ViewState side.
	history       []historyEntry
	keyPrefix     string
	notify        notify.Model
	notifyLog     *notifylog.Log
	notifyHistory notifyhistory.Model
	switcher      switcher
	gate          *reloadgate.ReloadGate

	// conn is shared by Model copies and the About provider; go through
	// serverConn and setServerConn.
	conn   *atomic.Pointer[ServerConn]
	stream serverStream
	store  storeWatch
}

func New(repo git.Repo, settings Settings) Model {
	m := Model{
		repo:          repo,
		settings:      settings,
		navState:      navstate.NewState(settings.ActiveWorktreePath),
		livePageByTab: make(map[nav.TabID]livePage),
		notify:        notify.New(settings.UseNerdFontIcons),
		notifyLog:     notifylog.New(),
		notifyHistory: notifyhistory.New(),
		gate:          reloadgate.New(),
		conn:          &atomic.Pointer[ServerConn]{},
	}
	m.settings.About = aboutLines(m.conn, settings.Terminal)
	if settings.Server == nil {
		m.setServerConn(ServerConn{State: ServerDown})
	}
	if m.settings.InitialRoute.Tab == "" {
		m.settings.InitialRoute = nav.ViewState{Tab: nav.TabWorktrees}
	}
	m.navState.SetInitialTab(m.settings.InitialRoute)
	initialRoute := m.navState.Active()
	// No snapshot has arrived yet, so the page has nothing to reply to.
	page, _ := m.newLivePage(initialRoute)
	page.didInit = true
	page.viewState = initialRoute
	m.livePageByTab[initialRoute.Tab] = page
	m.gate.MarkLoaded(initialRoute.Tab)
	return m
}

func (m Model) Init() tea.Cmd {
	if m.settings.Server == nil {
		// Permanently down: the store watch starts with the program.
		return tea.Batch(m.activePage().model.Init(), func() tea.Msg { return storeWatchStartMsg{} })
	}
	return tea.Batch(m.activePage().model.Init(), m.cmdServerProbe())
}

func (m Model) newTicketsModel(root string, s ui.Settings) ticketsui.Model {
	tm := ticketsui.NewModel(root, s, keys.New(Bindings()))
	if m.settings.Server != nil {
		tm = tm.WithServer(m.settings.Server.Client).WithCwdProject(cwdProjectName(root))
	}
	return tm
}

// cwdProjectName is the registered project's address name for root's repo, or
// "" when root is not in a registered project.
func cwdProjectName(root string) string {
	dir, err := gxtickets.RootFor(root)
	if err != nil {
		return ""
	}
	return gxtickets.ProjectName(dir)
}

func (m Model) newQueueModel(root string, s ui.Settings) ticketsui.QueueModel {
	qm := ticketsui.NewQueueModel(root, s, keys.New(Bindings()))
	if m.settings.Server != nil {
		qm = qm.WithServerLink(m.settings.Server.Client, m.settings.Server.Start)
	}
	return qm
}

func (m Model) newAgentsModel(s ui.Settings) agentsui.Model {
	if m.settings.Server == nil {
		return agentsui.NewModel(nil, s)
	}
	return agentsui.NewModel(m.settings.Server.Client, s).WithLastViewedFile(agentsui.LastViewedFile()).
		WithCwdProject(cwdProjectName(m.settings.ActiveWorktreePath))
}

func (m Model) View() tea.View {
	child := m.activePage().model.View()
	content := child.Content
	if strings.TrimSpace(content) == "" {
		content = "\n"
	}
	content = m.withChrome(content)
	if m.keyPrefix != "" {
		hints := hintsForPrefix(m.keyPrefix)
		if source, ok := m.activePage().model.(ui.ChordHintSource); ok {
			km := source.KeyManager()
			hints = append(hints, ui.ChordBindingsFromHints(km.HintsForPrefix(m.keyPrefix))...)
		}
		if len(hints) > 0 {
			content = ui.OverlayBottomRight(content, ui.RenderChordOverlay(m.keyPrefix, hints), m.width, m.height)
		}
	}
	if m.notifyHistory.IsOpen {
		content = m.notifyHistory.View(content, m.width, m.height)
	}
	if stack := m.notify.View(); stack != "" {
		content = ui.OverlayTopRightMargin(content, stack, m.width, 1, ui.ChromeRows)
	}
	if m.switcher.isOpen {
		content = ui.PlaceOverlay(content, m.renderSwitcher(), switcherX, ui.ChromeRows)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = child.MouseMode
	v.ReportFocus = child.ReportFocus
	// The switcher has no cursor of its own; the page's would blink through it.
	if child.Cursor != nil && !m.switcher.isOpen {
		cursor := *child.Cursor
		cursor.Y += ui.ChromeRows
		v.Cursor = &cursor
	}
	v.OnMouse = child.OnMouse
	return v
}

// withChrome normalizes the page to the rows it was given and puts the title
// row and the blank row on top.
func (m Model) withChrome(page string) string {
	pageHeight := 0
	if m.height > 0 {
		pageHeight = m.childWindowSizeMsg().Height
	}
	page = normalizeFrameContent(page, m.width, pageHeight)
	lines := append([]string{m.renderTitleRow(m.width).line, m.blankChromeRow(m.width)}, strings.Split(page, "\n")...)
	if m.height > 0 && len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

func (m Model) activePage() historyEntry {
	if len(m.history) > 0 {
		return m.history[len(m.history)-1]
	}
	activeTab := m.navState.ActiveTab()
	viewState := m.navState.Active()
	page := m.livePageByTab[activeTab]
	return historyEntry{
		viewState: viewState,
		model:     page.model,
	}
}

func (m *Model) setActivePage(page historyEntry) {
	if len(m.history) > 0 {
		m.history[len(m.history)-1] = page
		return
	}
	activeTab := m.navState.ActiveTab()
	live := m.livePageByTab[activeTab]
	live.model = page.model
	m.livePageByTab[activeTab] = live
}

func normalizeFrameContent(content string, targetWidth, targetHeight int) string {
	if targetWidth <= 0 && targetHeight <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if targetWidth > 0 {
		for i, line := range lines {
			line = ansi.Truncate(line, targetWidth, "")
			lineW := ansi.StringWidth(line)
			if lineW < targetWidth {
				line += strings.Repeat(" ", targetWidth-lineW)
			}
			lines[i] = line
		}
	}
	if targetHeight > 0 {
		if len(lines) > targetHeight {
			lines = lines[:targetHeight]
		}
		padLine := ""
		if targetWidth > 0 {
			padLine = strings.Repeat(" ", targetWidth)
		}
		for len(lines) < targetHeight {
			lines = append(lines, padLine)
		}
	}
	return strings.Join(lines, "\n")
}

// newHistoryEntry builds a page and, if it uses server state, hands it the
// current state (or the "no snapshot yet" marker). The command is the page's
// reply to that state, such as the Agents tab's attach dial, and must run.
func (m Model) newHistoryEntry(viewState nav.ViewState) (historyEntry, tea.Cmd) {
	entry := m.buildHistoryEntry(viewState)
	var cmd tea.Cmd
	entry.model, cmd = m.withServerState(entry.model)
	return entry, cmd
}

func (m Model) buildHistoryEntry(viewState nav.ViewState) historyEntry {
	s := m.settings.Settings
	s.EnableNavigation = true
	switch viewState.Tab {
	case nav.TabStatus:
		return historyEntry{
			viewState: viewState,
			model:     statusui.NewModel(viewState.WorktreeRoot, m.repo.IsBare, s, viewState.InitialPath, keys.New(Bindings())),
		}
	case nav.TabLog:
		return historyEntry{
			viewState: viewState,
			model: logui.NewModel(viewState.WorktreeRoot, viewState.Ref, s, logui.LogFilter{
				Path:      viewState.FilterPath,
				StartLine: viewState.FilterStartLine,
				EndLine:   viewState.FilterEndLine,
			}, keys.New(Bindings())),
		}
	case nav.TabStash:
		return historyEntry{
			viewState: viewState,
			model:     stashlistui.NewModel(viewState.WorktreeRoot, s, keys.New(Bindings())),
		}
	case nav.TabPRs:
		return historyEntry{
			viewState: viewState,
			model:     prsui.NewModelWithScope(viewState.WorktreeRoot, s, keys.New(Bindings()), viewState.AllRepos),
		}
	case nav.TabTickets:
		return historyEntry{
			viewState: viewState,
			model:     m.newTicketsModel(viewState.WorktreeRoot, s),
		}
	case nav.TabQueue:
		return historyEntry{
			viewState: viewState,
			model:     m.newQueueModel(viewState.WorktreeRoot, s),
		}
	case nav.TabAgents:
		return historyEntry{
			viewState: viewState,
			model:     m.newAgentsModel(s),
		}
	case nav.TabWorktrees:
		fallthrough
	default:
		return historyEntry{
			viewState: nav.ViewState{Tab: nav.TabWorktrees},
			model:     worktrees.NewWithSettings(m.repo, m.settings.ActiveWorktreePath, s, keys.New(Bindings())),
		}
	}
}

func (m Model) childWindowSizeMsg() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: m.width, Height: max(1, m.height-ui.ChromeRows)}
}

func (m Model) resizeCurrentCmd() tea.Cmd {
	if m.width <= 0 || m.height <= 0 {
		return nil
	}
	// The terminal size, not the page size: Update reads every WindowSizeMsg as
	// a terminal resize and takes the chrome off itself.
	size := tea.WindowSizeMsg{Width: m.width, Height: m.height}
	return func() tea.Msg {
		return size
	}
}

func (m *Model) restoreLogSelectionFromPoppedPage(popped historyEntry) {
	commitModel, ok := popped.model.(commitui.Model)
	if !ok {
		return
	}
	ref := commitModel.CurrentRef()
	if ref == "" {
		return
	}
	current := m.activePage()
	if current.viewState.Tab != nav.TabLog {
		return
	}
	logModel, ok := current.model.(logui.Model)
	if !ok {
		return
	}
	current.model = logModel.SelectRef(ref)
	m.setActivePage(current)
	// Keep router tab memory in sync so future tab switches restore the correct ref.
	m.navState.ApplyViewStateChanged(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: current.viewState.WorktreeRoot, Ref: ref})
}
