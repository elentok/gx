package nav

import tea "charm.land/bubbletea/v2"

type TabID string

const (
	TabWorktrees TabID = "worktrees"
	TabLog       TabID = "log"
	TabStatus    TabID = "status"
	TabStash     TabID = "stash"
	TabPRs       TabID = "prs"
	TabTickets   TabID = "tickets"
	TabQueue     TabID = "queue"
	TabAgents    TabID = "agents"
)

type ViewState struct {
	Tab          TabID
	WorktreeRoot string
	Ref          string
	InitialPath  string
	FocusSubject string
	AllRepos     bool

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

type openSwitcherMsg struct{}

type backMsg struct{}

// forceQuitMsg is ctrl+c's "quit right now, regardless of nav depth" signal —
// distinct from backMsg, which unwinds the history stack one level at a time.
type forceQuitMsg struct{}

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

// OpenSwitcher asks the app to open the view switcher, as ctrl+o does.
func OpenSwitcher() tea.Cmd {
	return func() tea.Msg {
		return openSwitcherMsg{}
	}
}

func IsOpenSwitcher(msg tea.Msg) bool {
	_, ok := msg.(openSwitcherMsg)
	return ok
}

type openHelpMsg struct{}

// OpenHelp asks the active page to open its help, as its "?" key does. It is
// a message rather than a synthetic "?" key so a focused input or the agents
// terminal never receives the keystroke.
func OpenHelp() tea.Cmd {
	return func() tea.Msg {
		return openHelpMsg{}
	}
}

func IsOpenHelp(msg tea.Msg) bool {
	_, ok := msg.(openHelpMsg)
	return ok
}

// EndSearch is how every page treats a focused search input on OpenHelp:
// opening help ends the search, and closing help does not restore it. It
// replays Esc through the page's own Update, so the search ends exactly as
// the user's Esc would end it, match clearing and all.
func EndSearch[M tea.Model](page M, searching bool) (M, tea.Cmd) {
	if !searching {
		return page, nil
	}
	next, cmd := page.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	return next.(M), cmd
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

// ForceQuit signals ctrl+c's unconditional-quit intent, still subject to the
// app shell's quit guard (see handleForceQuit) — the same guard handleBack
// applies, but without unwinding the nav stack first.
func ForceQuit() tea.Cmd {
	return func() tea.Msg {
		return forceQuitMsg{}
	}
}

func IsForceQuit(msg tea.Msg) bool {
	_, ok := msg.(forceQuitMsg)
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
