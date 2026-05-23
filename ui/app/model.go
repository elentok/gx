package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	commitui "github.com/elentok/gx/ui/commit"
	"github.com/elentok/gx/ui/keys"
	logui "github.com/elentok/gx/ui/log"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	statusui "github.com/elentok/gx/ui/status"
	"github.com/elentok/gx/ui/worktrees"
)

type Settings struct {
	InitialRoute       nav.ViewState
	ActiveWorktreePath string
	ui.Settings
}

type historyEntry struct {
	route nav.ViewState
	model tea.Model
}

type livePage struct {
	model   tea.Model
	didInit bool
}

type Model struct {
	repo     git.Repo
	settings Settings
	router   routerState

	width  int
	height int

	activeTab      nav.TabID
	lastRouteByTab map[nav.TabID]nav.ViewState
	livePageByTab  map[nav.TabID]livePage
	histories      map[nav.TabID][]historyEntry
	history        []historyEntry
	keyPrefix      string
	notify         notify.Model
}

func New(repo git.Repo, settings Settings) Model {
	m := Model{
		repo:           repo,
		settings:       settings,
		lastRouteByTab: make(map[nav.TabID]nav.ViewState),
		livePageByTab:  make(map[nav.TabID]livePage),
		histories:      make(map[nav.TabID][]historyEntry),
		notify:         notify.New(settings.UseNerdFontIcons),
	}
	if m.settings.InitialRoute.Tab == "" {
		m.settings.InitialRoute = nav.ViewState{Tab: nav.TabWorktrees}
	}
	m.router = newRouterState(m.settings.InitialRoute, m.settings.ActiveWorktreePath)
	m.activeTab = m.router.activeTab
	m.ensureTabs()
	initialRoute := m.tabRouteForRoute(m.settings.InitialRoute)
	page := m.newLivePage(initialRoute)
	page.didInit = true
	m.lastRouteByTab[m.activeTab] = initialRoute
	m.livePageByTab[m.activeTab] = page
	if m.settings.InitialRoute.Tab == nav.TabCommit {
		m.history = append(m.history, m.newHistoryEntry(m.settings.InitialRoute))
		m.histories[m.activeTab] = m.history
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return m.activePage().model.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var notifyCmd tea.Cmd
	m.notify, notifyCmd = m.notify.Update(msg)

	if route, ok := nav.IsSwitch(msg); ok {
		next, cmd := m.switchTab(route)
		return next, tea.Batch(notifyCmd, cmd)
	}
	if route, ok := nav.IsOpen(msg); ok {
		next := m.newHistoryEntry(route)
		m.router.push(route)
		m.history = append(m.history, next)
		return m, tea.Batch(notifyCmd, tea.ClearScreen, next.model.Init(), m.resizeCurrentCmd())
	}
	if route, ok := nav.IsViewStateChanged(msg); ok {
		m.applyRouteChanged(route)
		return m, notifyCmd
	}
	if nav.IsBack(msg) {
		_, ok := m.router.back()
		if !ok || len(m.history) == 0 {
			return m, tea.Quit
		}
		popped := m.history[len(m.history)-1]
		m.history = m.history[:len(m.history)-1]
		m.restoreLogSelectionFromPoppedPage(popped)
		return m, tea.Batch(notifyCmd, tea.ClearScreen, m.resizeCurrentCmd())
	}

	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		msg = m.childWindowSizeMsg()
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		type inputFocuser interface{ InputFocused() bool }
		active := m.activePage().model
		if f, ok := active.(inputFocuser); !ok || !f.InputFocused() {
			if handled, cmd := m.handleShellChordKey(key); handled {
				return m, tea.Batch(notifyCmd, cmd)
			}
		}
	}

	current := m.activePage()
	nextModel, cmd := current.model.Update(msg)
	current.model = nextModel
	m.setActivePage(current)
	return m, tea.Batch(notifyCmd, cmd)
}

func (m Model) View() tea.View {
	child := m.activePage().model.View()
	content := child.Content
	if strings.TrimSpace(content) == "" {
		content = "\n"
	}
	content = normalizeFrameContent(content, m.width, m.height)
	content = injectTabsIntoFooter(content, m.tabsView(), m.width)
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
	if stack := m.notify.View(); stack != "" {
		content = ui.OverlayTopRightMargin(content, stack, m.width, 1, 1)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = child.MouseMode
	v.ReportFocus = child.ReportFocus
	v.Cursor = child.Cursor
	v.OnMouse = child.OnMouse
	return v
}

func (m Model) activePage() historyEntry {
	if len(m.history) > 0 {
		return m.history[len(m.history)-1]
	}
	route := m.lastRouteByTab[m.activeTab]
	page := m.livePageByTab[m.activeTab]
	return historyEntry{
		route: route,
		model: page.model,
	}
}

func (m *Model) setActivePage(page historyEntry) {
	if len(m.history) > 0 {
		m.history[len(m.history)-1] = page
		return
	}
	live := m.livePageByTab[m.activeTab]
	live.model = page.model
	m.livePageByTab[m.activeTab] = live
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

func (m Model) newHistoryEntry(route nav.ViewState) historyEntry {
	s := m.settings.Settings
	s.EnableNavigation = true
	switch route.Tab {
	case nav.TabStatus:
		return historyEntry{
			route: route,
			model: statusui.NewModel(route.WorktreeRoot, s, route.InitialPath, keys.New(Bindings())),
		}
	case nav.TabLog:
		return historyEntry{
			route: route,
			model: logui.NewModel(route.WorktreeRoot, route.Ref, s, logui.LogFilter{
				Path:      route.FilterPath,
				StartLine: route.FilterStartLine,
				EndLine:   route.FilterEndLine,
			}, keys.New(Bindings())),
		}
	case nav.TabCommit:
		return historyEntry{
			route: route,
			model: commitui.NewModel(route.WorktreeRoot, route.Ref, route.FilterPath, s, keys.New(Bindings())),
		}
	case nav.TabWorktrees:
		fallthrough
	default:
		return historyEntry{
			route: nav.ViewState{Tab: nav.TabWorktrees},
			model: worktrees.NewWithSettings(m.repo, m.settings.ActiveWorktreePath, s),
		}
	}
}

func (m Model) childWindowSizeMsg() tea.WindowSizeMsg {
	height := m.height
	if height < 1 {
		height = 1
	}
	return tea.WindowSizeMsg{Width: m.width, Height: height}
}

func (m Model) resizeCurrentCmd() tea.Cmd {
	if m.width <= 0 || m.height <= 0 {
		return nil
	}
	size := m.childWindowSizeMsg()
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
	if current.route.Tab != nav.TabLog {
		return
	}
	logModel, ok := current.model.(logui.Model)
	if !ok {
		return
	}
	current.model = logModel.SelectRef(ref)
	m.setActivePage(current)
}

func (m *Model) applyRouteChanged(route nav.ViewState) {
	tabRoute := m.tabRouteForRoute(route)
	m.router.routeChanged(route, m.settings.ActiveWorktreePath)
	m.ensureTabs()
	m.lastRouteByTab[tabRoute.Tab] = tabRoute
}
