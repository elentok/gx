package nav

import tea "charm.land/bubbletea/v2"

type TabID string

const (
	TabWorktrees TabID = "worktrees"
	TabLog       TabID = "log"
	TabStatus    TabID = "status"
	TabCommit    TabID = "commit"
)

type ViewState struct {
	Tab          TabID
	WorktreeRoot string
	Ref          string
	InitialPath  string
	FocusSubject string

	FilterPath      string
	FilterStartLine int
	FilterEndLine   int
}

type ViewStateProvider interface {
	CurrentViewState() (ViewState, bool)
}

type openMsg struct {
	ViewState ViewState
}

type switchMsg struct {
	ViewState ViewState
}

type backMsg struct{}

type routeChangedMsg struct {
	ViewState ViewState
}

func Open(route ViewState) tea.Cmd {
	return func() tea.Msg {
		return openMsg{ViewState: route}
	}
}

func Switch(route ViewState) tea.Cmd {
	return func() tea.Msg {
		return switchMsg{ViewState: route}
	}
}

func Back() tea.Cmd {
	return func() tea.Msg {
		return backMsg{}
	}
}

func ViewStateChanged(route ViewState) tea.Cmd {
	return func() tea.Msg {
		return routeChangedMsg{ViewState: route}
	}
}

func IsOpen(msg tea.Msg) (ViewState, bool) {
	open, ok := msg.(openMsg)
	return open.ViewState, ok
}

func IsSwitch(msg tea.Msg) (ViewState, bool) {
	switchTo, ok := msg.(switchMsg)
	return switchTo.ViewState, ok
}

func IsBack(msg tea.Msg) bool {
	_, ok := msg.(backMsg)
	return ok
}

func IsViewStateChanged(msg tea.Msg) (ViewState, bool) {
	changed, ok := msg.(routeChangedMsg)
	return changed.ViewState, ok
}
