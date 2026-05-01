package app

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

type placeholderModel struct {
	title  string
	body   string
	route  nav.Route
	width  int
	height int
	ready  bool
	keyG   bool
}

func newPlaceholderModel(title, body string, route nav.Route) placeholderModel {
	return placeholderModel{title: title, body: body, route: route}
}

func (m placeholderModel) Init() tea.Cmd { return nil }

func (m placeholderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q", "esc":
			return m, nav.Back()
		case "L":
			return m, nil
		}
		if m.keyG {
			m.keyG = false
			switch msg.String() {
			case "w":
				return m, nav.Push(nav.Route{Kind: nav.RouteWorktrees})
			case "s":
				if strings.TrimSpace(m.route.WorktreeRoot) == "" {
					return m, nil
				}
				return m, nav.Push(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: m.route.WorktreeRoot})
			case "l":
				return m, nav.Push(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.route.WorktreeRoot, Ref: m.route.Ref})
			}
			return m, nil
		}
		if msg.String() == "g" {
			m.keyG = true
			return m, nil
		}
	}
	return m, nil
}

func (m placeholderModel) View() tea.View {
	if !m.ready {
		return tea.NewView("\n  Initializing…")
	}
	lines := []string{
		ui.StyleHeading.Render(m.title),
		"",
		ui.StyleBody.Render(m.body),
	}
	if strings.TrimSpace(m.route.WorktreeRoot) != "" {
		lines = append(lines, "", ui.StyleMuted.Render(fmt.Sprintf("worktree: %s", m.route.WorktreeRoot)))
	}
	if strings.TrimSpace(m.route.Ref) != "" {
		lines = append(lines, ui.StyleMuted.Render(fmt.Sprintf("ref: %s", m.route.Ref)))
	}
	lines = append(lines, "", ui.StyleHint.Render("gw worktrees · gl log · gs status · q back"))
	return tea.NewView(strings.Join(lines, "\n"))
}
