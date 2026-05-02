package nav

import tea "charm.land/bubbletea/v2"

type RouteKind string

const (
	RouteWorktrees RouteKind = "worktrees"
	RouteLog       RouteKind = "log"
	RouteStatus    RouteKind = "status"
	RouteCommit    RouteKind = "commit"
)

type Route struct {
	Kind         RouteKind
	WorktreeRoot string
	Ref          string
	InitialPath  string
}

type pushMsg struct {
	Route Route
}

type replaceMsg struct {
	Route Route
}

type backMsg struct{}

func Push(route Route) tea.Cmd {
	return func() tea.Msg {
		return pushMsg{Route: route}
	}
}

func Replace(route Route) tea.Cmd {
	return func() tea.Msg {
		return replaceMsg{Route: route}
	}
}

func Back() tea.Cmd {
	return func() tea.Msg {
		return backMsg{}
	}
}

func IsPush(msg tea.Msg) (Route, bool) {
	push, ok := msg.(pushMsg)
	return push.Route, ok
}

func IsReplace(msg tea.Msg) (Route, bool) {
	replace, ok := msg.(replaceMsg)
	return replace.Route, ok
}

func IsBack(msg tea.Msg) bool {
	_, ok := msg.(backMsg)
	return ok
}
