package nav

import tea "charm.land/bubbletea/v2"

type TabID string

const (
	TabWorktrees TabID = "worktrees"
	TabLog       TabID = "log"
	TabStatus    TabID = "status"
	TabStash     TabID = "stash"
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

type ViewContext struct {
	Tab          TabID
	WorktreeRoot string
	Ref          string
	InitialPath  string
}

type ViewOptions struct {
	FocusSubject string
	FilterPath   string

	FilterStartLine int
	FilterEndLine   int
}

func (s ViewState) Context() ViewContext {
	return ViewContext{
		Tab:          s.Tab,
		WorktreeRoot: s.WorktreeRoot,
		Ref:          s.Ref,
		InitialPath:  s.InitialPath,
	}
}

func (s ViewState) Options() ViewOptions {
	return ViewOptions{
		FocusSubject:    s.FocusSubject,
		FilterPath:      s.FilterPath,
		FilterStartLine: s.FilterStartLine,
		FilterEndLine:   s.FilterEndLine,
	}
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

type viewStateChangedMsg struct {
	ViewState ViewState
}

type repoMutatedMsg struct{}

func Open(vs ViewState) tea.Cmd {
	return func() tea.Msg {
		return openMsg{ViewState: vs}
	}
}

func Switch(vs ViewState) tea.Cmd {
	return func() tea.Msg {
		return switchMsg{ViewState: vs}
	}
}

func Back() tea.Cmd {
	return func() tea.Msg {
		return backMsg{}
	}
}

func ViewStateChanged(vs ViewState) tea.Cmd {
	return func() tea.Msg {
		return viewStateChangedMsg{ViewState: vs}
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
	changed, ok := msg.(viewStateChangedMsg)
	return changed.ViewState, ok
}

func RepoMutated() tea.Cmd {
	return func() tea.Msg {
		return repoMutatedMsg{}
	}
}

func IsRepoMutated(msg tea.Msg) bool {
	_, ok := msg.(repoMutatedMsg)
	return ok
}
