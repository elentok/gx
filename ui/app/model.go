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
	"github.com/elentok/gx/ui/navstate"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/reloadgate"
	stashlistui "github.com/elentok/gx/ui/stashlist"
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
	model     tea.Model
	didInit   bool
	viewState nav.ViewState // the ViewState when this page was last created or re-routed
}

type Model struct {
	repo     git.Repo
	settings Settings

	width  int
	height int

	navState      navstate.State
	livePageByTab map[nav.TabID]livePage
	// history is the model side of the global deep-navigation stack.
	// navState.State holds the parallel ViewState side.
	history   []historyEntry
	keyPrefix string
	notify    notify.Model
	gate      *reloadgate.ReloadGate
}

func New(repo git.Repo, settings Settings) Model {
	m := Model{
		repo:          repo,
		settings:      settings,
		navState:      navstate.NewState(settings.ActiveWorktreePath),
		livePageByTab: make(map[nav.TabID]livePage),
		notify:        notify.New(settings.UseNerdFontIcons),
		gate:          reloadgate.New(),
	}
	if m.settings.InitialRoute.Tab == "" {
		m.settings.InitialRoute = nav.ViewState{Tab: nav.TabWorktrees}
	}
	m.navState.SetInitialTab(m.settings.InitialRoute)
	initialRoute := m.navState.Active()
	page := m.newLivePage(initialRoute)
	page.didInit = true
	page.viewState = initialRoute
	m.livePageByTab[initialRoute.Tab] = page
	m.gate.MarkLoaded(initialRoute.Tab)
	return m
}

func (m Model) Init() tea.Cmd {
	return m.activePage().model.Init()
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
	activeTab := m.navState.ActiveTab()
	viewState := m.navState.Active()
	page := m.livePageByTab[activeTab]
	return historyEntry{
		viewState: viewState,
		model:     page.model,
	}
}

func (m *Model) setActivePage(page historyEntry) {
	if len(m.history) > 0 {
		m.history[len(m.history)-1] = page
		return
	}
	activeTab := m.navState.ActiveTab()
	live := m.livePageByTab[activeTab]
	live.model = page.model
	m.livePageByTab[activeTab] = live
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
	case nav.TabStash:
		return historyEntry{
			viewState: viewState,
			model:     stashlistui.NewTab(viewState.WorktreeRoot, s, keys.New(Bindings())),
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
	// Keep router tab memory in sync so future tab switches restore the correct ref.
	m.navState.ApplyViewStateChanged(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: current.viewState.WorktreeRoot, Ref: ref})
}

