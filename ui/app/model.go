package app

import (
	"strings"

	"github.com/elentok/gx/git"
	commitui "github.com/elentok/gx/ui/commit"
	logui "github.com/elentok/gx/ui/log"
	"github.com/elentok/gx/ui/nav"
	statusui "github.com/elentok/gx/ui/status"
	"github.com/elentok/gx/ui/worktrees"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type Settings struct {
	InitialRoute       nav.Route
	ActiveWorktreePath string
	Commit             commitui.Settings
	Log                logui.Settings
	Worktrees          worktrees.Settings
	Status             statusui.Settings
}

type pageState struct {
	route nav.Route
	model tea.Model
}

type tabPageState struct {
	kind         nav.RouteKind
	worktreeRoot string
	ref          string
	initialPath  string
	model        tea.Model
	initialized  bool
}

type Model struct {
	repo     git.Repo
	settings Settings

	width  int
	height int

	activeTab nav.RouteKind
	tabs      map[nav.RouteKind]tabPageState
	histories map[nav.RouteKind][]pageState
	history   []pageState
	keyPrefix string
}

func New(repo git.Repo, settings Settings) Model {
	m := Model{
		repo:     repo,
		settings: settings,
		tabs:     make(map[nav.RouteKind]tabPageState),
		histories: make(map[nav.RouteKind][]pageState),
	}
	if m.settings.InitialRoute.Kind == "" {
		m.settings.InitialRoute = nav.Route{Kind: nav.RouteWorktrees}
	}
	m.activeTab = tabForRoute(m.settings.InitialRoute.Kind)
	m.ensureTabs()
	page := m.newTabPage(m.tabStateForRoute(m.settings.InitialRoute))
	page.initialized = true
	m.tabs[m.activeTab] = page
	if m.settings.InitialRoute.Kind == nav.RouteCommit {
		m.history = append(m.history, m.newPage(m.settings.InitialRoute))
		m.histories[m.activeTab] = m.history
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return m.activePage().model.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if route, ok := nav.IsReplace(msg); ok {
		return m.switchTab(route)
	}
	if route, ok := nav.IsPush(msg); ok {
		next := m.newPage(route)
		m.history = append(m.history, next)
		return m, tea.Batch(tea.ClearScreen, next.model.Init(), m.resizeCurrentCmd())
	}
	if nav.IsBack(msg) {
		if len(m.history) == 0 {
			return m, tea.Quit
		}
		popped := m.history[len(m.history)-1]
		m.history = m.history[:len(m.history)-1]
		m.restoreLogSelectionFromPoppedPage(popped)
		return m, tea.Batch(tea.ClearScreen, m.resizeCurrentCmd())
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
				return m, cmd
			}
		}
	}

	current := m.activePage()
	nextModel, cmd := current.model.Update(msg)
	current.model = nextModel
	m.setActivePage(current)
	return m, cmd
}

func (m Model) View() tea.View {
	child := m.activePage().model.View()
	content := child.Content
	if strings.TrimSpace(content) == "" {
		content = "\n"
	}
	content = normalizeFrameContent(content, m.width, m.height)
	content = injectTabsIntoFooter(content, m.tabsView(), m.width)

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = child.MouseMode
	v.ReportFocus = child.ReportFocus
	v.Cursor = child.Cursor
	v.OnMouse = child.OnMouse
	return v
}

func (m Model) activePage() pageState {
	if len(m.history) > 0 {
		return m.history[len(m.history)-1]
	}
	tab := m.tabs[m.activeTab]
	return pageState{
		route: nav.Route{
			Kind:         tab.kind,
			WorktreeRoot: tab.worktreeRoot,
			Ref:          tab.ref,
			InitialPath:  tab.initialPath,
		},
		model: tab.model,
	}
}

func (m *Model) setActivePage(page pageState) {
	if len(m.history) > 0 {
		m.history[len(m.history)-1] = page
		return
	}
	tab := m.tabs[m.activeTab]
	tab.model = page.model
	m.tabs[m.activeTab] = tab
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

func (m Model) newPage(route nav.Route) pageState {
	switch route.Kind {
	case nav.RouteStatus:
		settings := m.settings.Status
		settings.EnableNavigation = true
		if strings.TrimSpace(route.InitialPath) != "" {
			settings.InitialPath = route.InitialPath
		}
		return pageState{
			route: route,
			model: statusui.NewWithSettings(route.WorktreeRoot, settings),
		}
	case nav.RouteLog:
		settings := m.settings.Log
		settings.EnableNavigation = true
		return pageState{
			route: route,
			model: logui.NewWithSettings(route.WorktreeRoot, route.Ref, settings),
		}
	case nav.RouteCommit:
		settings := m.settings.Commit
		settings.EnableNavigation = true
		return pageState{
			route: route,
			model: commitui.NewWithSettings(route.WorktreeRoot, route.Ref, settings),
		}
	case nav.RouteWorktrees:
		fallthrough
	default:
		settings := m.settings.Worktrees
		settings.EnableNavigation = true
		return pageState{
			route: nav.Route{Kind: nav.RouteWorktrees},
			model: worktrees.NewWithSettings(m.repo, m.settings.ActiveWorktreePath, settings),
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

func (m *Model) restoreLogSelectionFromPoppedPage(popped pageState) {
	commitModel, ok := popped.model.(commitui.Model)
	if !ok {
		return
	}
	ref := commitModel.CurrentRef()
	if ref == "" {
		return
	}
	current := m.activePage()
	if current.route.Kind != nav.RouteLog {
		return
	}
	logModel, ok := current.model.(logui.Model)
	if !ok {
		return
	}
	current.model = logModel.SelectRef(ref)
	m.setActivePage(current)
}
