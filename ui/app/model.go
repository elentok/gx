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
	viewState nav.ViewState
	model     tea.Model
}

type livePage struct {
	model   tea.Model
	didInit bool
}

type Model struct {
	repo     git.Repo
	settings Settings

	width  int
	height int

	// liveTab is the base tab shown when the stack is empty.
	// activeTab mirrors the currently visible tab (top of stack, or liveTab).
	liveTab            nav.TabID
	activeTab          nav.TabID
	lastViewStateByTab map[nav.TabID]nav.ViewState
	livePageByTab      map[nav.TabID]livePage
	// stack is the single global deep-navigation stack.
	// Switch() clears it; Open() pushes; Back() pops.
	stack     []historyEntry
	keyPrefix string
	notify    notify.Model
}

func New(repo git.Repo, settings Settings) Model {
	m := Model{
		repo:               repo,
		settings:           settings,
		lastViewStateByTab: make(map[nav.TabID]nav.ViewState),
		livePageByTab:      make(map[nav.TabID]livePage),
		notify:             notify.New(settings.UseNerdFontIcons),
	}
	if m.settings.InitialRoute.Tab == "" {
		m.settings.InitialRoute = nav.ViewState{Tab: nav.TabWorktrees}
	}
	initialRoute := m.tabViewStateForViewContext(m.settings.InitialRoute.Context())
	m.liveTab = initialRoute.Tab
	m.activeTab = m.liveTab
	m.ensureTabs()
	m.lastViewStateByTab[m.activeTab] = initialRoute
	page := m.newLivePage(initialRoute)
	page.didInit = true
	m.livePageByTab[m.activeTab] = page
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
		entry := m.newHistoryEntry(route)
		m.stack = append(m.stack, entry)
		m.activeTab = route.Tab
		m.lastViewStateByTab[route.Tab] = route
		return m, tea.Batch(notifyCmd, tea.ClearScreen, entry.model.Init(), m.resizeCurrentCmd())
	}
	if route, ok := nav.IsViewStateChanged(msg); ok {
		m.applyViewStateChanged(route)
		return m, notifyCmd
	}
	if nav.IsBack(msg) {
		if len(m.stack) == 0 {
			return m, tea.Quit
		}
		popped := m.stack[len(m.stack)-1]
		m.stack = m.stack[:len(m.stack)-1]
		m.restoreLogSelectionFromPoppedPage(popped)
		if len(m.stack) > 0 {
			m.activeTab = m.stack[len(m.stack)-1].viewState.Tab
		} else {
			m.activeTab = m.liveTab
		}
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
	prevViewState, prevOK := viewStateOf(current.model)
	nextModel, cmd := current.model.Update(msg)
	current.model = nextModel
	m.setActivePage(current)
	nextViewState, nextOK := viewStateOf(nextModel)
	cmd = nav.AppendViewStateChanged(cmd, m.settings.EnableNavigation, prevViewState, prevOK, nextViewState, nextOK)
	return m, tea.Batch(notifyCmd, cmd)
}

func viewStateOf(model tea.Model) (nav.ViewState, bool) {
	if vsp, ok := model.(nav.ViewStateProvider); ok {
		return vsp.CurrentViewState()
	}
	return nav.ViewState{}, false
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
	if len(m.stack) > 0 {
		return m.stack[len(m.stack)-1]
	}
	viewState := m.lastViewStateByTab[m.activeTab]
	page := m.livePageByTab[m.activeTab]
	return historyEntry{
		viewState: viewState,
		model:     page.model,
	}
}

func (m *Model) setActivePage(page historyEntry) {
	if len(m.stack) > 0 {
		m.stack[len(m.stack)-1] = page
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

func (m Model) newHistoryEntry(viewState nav.ViewState) historyEntry {
	s := m.settings.Settings
	s.EnableNavigation = true
	switch viewState.Tab {
	case nav.TabStatus:
		return historyEntry{
			viewState: viewState,
			model:     statusui.NewModel(viewState.WorktreeRoot, s, viewState.InitialPath, keys.New(Bindings())),
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
	case nav.TabCommit:
		return historyEntry{
			viewState: viewState,
			model:     commitui.NewModel(viewState.WorktreeRoot, viewState.Ref, viewState.FilterPath, s, keys.New(Bindings())),
		}
	case nav.TabWorktrees:
		fallthrough
	default:
		return historyEntry{
			viewState: nav.ViewState{Tab: nav.TabWorktrees},
			model:     worktrees.NewWithSettings(m.repo, m.settings.ActiveWorktreePath, s),
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
	if current.viewState.Tab != nav.TabLog {
		return
	}
	logModel, ok := current.model.(logui.Model)
	if !ok {
		return
	}
	current.model = logModel.SelectRef(ref)
	m.setActivePage(current)
	// Keep lastViewStateByTab in sync so future tab switches restore the correct ref.
	remembered := m.lastViewStateByTab[nav.TabLog]
	remembered.Ref = ref
	m.lastViewStateByTab[nav.TabLog] = remembered
}

func (m *Model) applyViewStateChanged(viewState nav.ViewState) {
	tabViewState := m.tabViewStateForViewContext(viewState.Context())
	m.ensureTabs()
	m.lastViewStateByTab[tabViewState.Tab] = tabViewState
	// If the top of the stack is for the same tab, keep it current.
	if len(m.stack) > 0 && m.stack[len(m.stack)-1].viewState.Tab == tabViewState.Tab {
		m.stack[len(m.stack)-1].viewState = tabViewState
	}
}
