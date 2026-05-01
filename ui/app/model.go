package app

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/nav"
	statusui "github.com/elentok/gx/ui/status"
	"github.com/elentok/gx/ui/worktrees"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type Settings struct {
	InitialRoute       nav.Route
	ActiveWorktreePath string
	Worktrees          worktrees.Settings
	Status             statusui.Settings
}

type pageState struct {
	route nav.Route
	model tea.Model
}

type Model struct {
	repo     git.Repo
	settings Settings

	width  int
	height int

	current pageState
	history []pageState
}

func New(repo git.Repo, settings Settings) Model {
	m := Model{repo: repo, settings: settings}
	if m.settings.InitialRoute.Kind == "" {
		m.settings.InitialRoute = nav.Route{Kind: nav.RouteWorktrees}
	}
	m.current = m.newPage(m.settings.InitialRoute)
	return m
}

func (m Model) Init() tea.Cmd {
	return m.current.model.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if route, ok := nav.IsPush(msg); ok {
		next := m.newPage(route)
		m.history = append(m.history, m.current)
		m.current = next
		return m, tea.Batch(m.current.model.Init(), m.resizeCurrentCmd())
	}
	if nav.IsBack(msg) {
		if len(m.history) == 0 {
			return m, tea.Quit
		}
		m.current = m.history[len(m.history)-1]
		m.history = m.history[:len(m.history)-1]
		return m, m.resizeCurrentCmd()
	}

	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		msg = m.childWindowSizeMsg()
	}

	nextModel, cmd := m.current.model.Update(msg)
	m.current.model = nextModel
	return m, cmd
}

func (m Model) View() tea.View {
	child := m.current.model.View()
	content := child.Content
	if strings.TrimSpace(content) == "" {
		content = "\n"
	}
	content = lipgloss.JoinVertical(lipgloss.Left, content, m.tabsView())

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = child.MouseMode
	v.ReportFocus = child.ReportFocus
	v.Cursor = child.Cursor
	v.OnMouse = child.OnMouse
	return v
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
		return pageState{
			route: route,
			model: newPlaceholderModel("Log", "Log page not implemented yet.", route),
		}
	case nav.RouteCommit:
		return pageState{
			route: route,
			model: newPlaceholderModel("Commit", "Commit page not implemented yet.", route),
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
	height := m.height - 1
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
