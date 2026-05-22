package nav

import tea "charm.land/bubbletea/v2"

type TabID string

const (
	TabWorktrees TabID = "worktrees"
	TabLog       TabID = "log"
	TabStatus    TabID = "status"
	TabCommit    TabID = "commit"
)

type Route struct {
	Tab          TabID
	WorktreeRoot string
	Ref          string
	InitialPath  string
	FocusSubject string

	FilterPath      string
	FilterStartLine int
	FilterEndLine   int
}

type openMsg struct {
	Route Route
}

type switchMsg struct {
	Route Route
}

type backMsg struct{}

type routeChangedMsg struct {
	Route Route
}

func Open(route Route) tea.Cmd {
	return func() tea.Msg {
		return openMsg{Route: route}
	}
}

func Switch(route Route) tea.Cmd {
	return func() tea.Msg {
		return switchMsg{Route: route}
	}
}

func Back() tea.Cmd {
	return func() tea.Msg {
		return backMsg{}
	}
}

func RouteChanged(route Route) tea.Cmd {
	return func() tea.Msg {
		return routeChangedMsg{Route: route}
	}
}

func IsOpen(msg tea.Msg) (Route, bool) {
	open, ok := msg.(openMsg)
	return open.Route, ok
}

func IsSwitch(msg tea.Msg) (Route, bool) {
	switchTo, ok := msg.(switchMsg)
	return switchTo.Route, ok
}

func IsBack(msg tea.Msg) bool {
	_, ok := msg.(backMsg)
	return ok
}

func IsRouteChanged(msg tea.Msg) (Route, bool) {
	changed, ok := msg.(routeChangedMsg)
	return changed.Route, ok
}
