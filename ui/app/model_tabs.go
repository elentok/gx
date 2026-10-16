package app

import (
	"path/filepath"

	logui "github.com/elentok/gx/ui/log"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/navstate"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) ensureLivePages() {
	for _, kind := range []nav.TabID{nav.TabWorktrees, nav.TabLog, nav.TabStatus, nav.TabStash, nav.TabPRs, nav.TabTickets, nav.TabQueue, nav.TabAgents} {
		if _, ok := m.livePageByTab[kind]; !ok {
			m.livePageByTab[kind] = livePage{}
		}
	}
}

// switchTab is called from handleShellChordKey (direct key dispatch, outside the nav message path).
func (m Model) switchTab(viewState nav.ViewState) (Model, tea.Cmd) {
	type modalOpener interface{ ModalOpen() bool }
	if mo, ok := m.activePage().model.(modalOpener); ok && mo.ModalOpen() {
		return m, notify.Info("close the modal first")
	}
	prev := m.navState.Active()
	tabVS := m.navState.Switch(viewState)
	return m.applySwitch(tabVS, prev)
}

// openNotifyHistory opens the "g n" notification-history modal over a
// snapshot of notifylog's current entries, matching switchTab's rule that
// deep page modals (e.g. the tickets tab's launch dialogs) must close first.
func (m Model) openNotifyHistory() (Model, tea.Cmd) {
	type modalOpener interface{ ModalOpen() bool }
	if mo, ok := m.activePage().model.(modalOpener); ok && mo.ModalOpen() {
		return m, notify.Info("close the modal first")
	}
	repoName := filepath.Base(m.repo.LinkedWorktreeDir())
	worktreeName := filepath.Base(m.settings.ActiveWorktreePath)
	m.notifyHistory = m.notifyHistory.Open(m.notifyLog.Entries(), repoName, worktreeName)
	return m, nil
}

// applySwitch activates tabVS and hands its page the current server state:
// hidden pages are not kept up to date, so every switch catches the page up.
func (m Model) applySwitch(tabVS, prevVS nav.ViewState) (Model, tea.Cmd) {
	m, cmd := m.switchToTab(tabVS, prevVS)
	m, deliver := m.deliverServerState()
	return m, tea.Batch(cmd, deliver)
}

func (m Model) switchToTab(tabVS, prevVS nav.ViewState) (Model, tea.Cmd) {
	// Derive outgoing model from model-side state: m.navState.activeTab has already been
	// updated by the pointer-receiver Switch call, so m.activePage() would return the
	// new page. Use the model-side stack or prevVS to find what the user was seeing.
	var outgoing tea.Model
	if len(m.history) > 0 {
		outgoing = m.history[len(m.history)-1].model
	} else {
		outgoing = m.livePageByTab[prevVS.Tab].model
	}

	// Clear model-side stack — tab switch exits the current deep-navigation session.
	m.history = nil
	m.ensureLivePages()

	currentPage := m.livePageByTab[tabVS.Tab]

	// Reconstruct when: no cached model, or the destination tab's view context changed
	// (e.g. different worktree or ref) since the cached model was created.
	if currentPage.model == nil || !navstate.SameViewContext(currentPage.viewState.Context(), tabVS.Context()) {
		var stateCmd tea.Cmd
		currentPage, stateCmd = m.newLivePage(tabVS)
		currentPage.didInit = true
		currentPage.viewState = tabVS
		m.livePageByTab[tabVS.Tab] = currentPage
		// Init loads fresh data; stamp the tab so it won't auto-reload on the next switch.
		m.gate.MarkLoaded(tabVS.Tab)
		return m, tea.Batch(onPageDeactivatedCmd(outgoing), currentPage.model.Init(), stateCmd, m.resizeCurrentCmd(), onPageActivatedCmd(currentPage.model))
	}
	if !currentPage.didInit {
		currentPage.didInit = true
		currentPage.viewState = tabVS
		m.livePageByTab[tabVS.Tab] = currentPage
		// Init loads fresh data; stamp the tab so it won't auto-reload on the next switch.
		m.gate.MarkLoaded(tabVS.Tab)
		return m, tea.Batch(onPageDeactivatedCmd(outgoing), currentPage.model.Init(), m.resizeCurrentCmd(), onPageActivatedCmd(currentPage.model))
	}
	if tabVS.FocusSubject != "" {
		if logModel, ok := currentPage.model.(logui.Model); ok {
			currentPage.model = logModel.WithPendingFocus(tabVS.FocusSubject)
			m.livePageByTab[tabVS.Tab] = currentPage
		}
		// Targeted navigation always reloads regardless of epoch.
		m.gate.MarkLoaded(tabVS.Tab)
		return m, tea.Batch(onPageDeactivatedCmd(outgoing), autoReloadCmd(currentPage.model), m.resizeCurrentCmd(), onPageActivatedCmd(currentPage.model))
	}
	// Reload when the tab is stale (a mutation occurred elsewhere) or when its
	// initial load never completed — e.g. the user left before the in-flight
	// reload returned, so its result was delivered to another page and the gate
	// was already optimistically stamped at switch-in. Without this the page
	// stays stuck on its loading state forever.
	if m.gate.ShouldAutoReload(tabVS.Tab) || needsInitialLoad(currentPage.model) {
		m.gate.MarkLoaded(tabVS.Tab)
		return m, tea.Batch(onPageDeactivatedCmd(outgoing), autoReloadCmd(currentPage.model), m.resizeCurrentCmd(), onPageActivatedCmd(currentPage.model))
	}
	return m, tea.Batch(onPageDeactivatedCmd(outgoing), m.resizeCurrentCmd(), onPageActivatedCmd(currentPage.model))
}

type pageInitialLoadAware interface {
	NeedsInitialLoad() bool
}

func needsInitialLoad(model tea.Model) bool {
	if a, ok := model.(pageInitialLoadAware); ok {
		return a.NeedsInitialLoad()
	}
	return false
}

type pageAutoReloadable interface {
	AutoReload() tea.Cmd
}

func autoReloadCmd(model tea.Model) tea.Cmd {
	if r, ok := model.(pageAutoReloadable); ok {
		return r.AutoReload()
	}
	return nil
}

type pageActivationAware interface {
	OnPageActivated() tea.Cmd
}

func onPageActivatedCmd(model tea.Model) tea.Cmd {
	if activator, ok := model.(pageActivationAware); ok {
		return activator.OnPageActivated()
	}
	return nil
}

type pageDeactivationAware interface {
	OnPageDeactivated() tea.Cmd
}

func onPageDeactivatedCmd(model tea.Model) tea.Cmd {
	if d, ok := model.(pageDeactivationAware); ok {
		return d.OnPageDeactivated()
	}
	return nil
}

// handleShellChordKey returns (newModel, cmd, handled) where handled indicates the key was consumed.
func (m Model) handleShellChordKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	key := msg.String()
	if m.keyPrefix == "g" {
		m.keyPrefix = ""
		switch key {
		case ",":
			next, cmd := m.switchRelativeTab(-1)
			return next, cmd, true
		case ".":
			next, cmd := m.switchRelativeTab(1)
			return next, cmd, true
		case "w":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabWorktrees})
			return next, cmd, true
		case "l":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabLog})
			return next, cmd, true
		case "s":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabStatus})
			return next, cmd, true
		case "S":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabStash})
			return next, cmd, true
		case "p":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabPRs})
			return next, cmd, true
		case "t":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabTickets})
			return next, cmd, true
		case "q":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabQueue})
			return next, cmd, true
		case "a":
			next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabAgents})
			return next, cmd, true
		case "n":
			next, cmd := m.openNotifyHistory()
			return next, cmd, true
		case "esc":
			return m, nil, true
		default:
			current := m.activePage()
			replayed, cmd := replayKeys(current.model, tea.KeyPressMsg{Code: 'g', Text: "g"}, msg)
			current.model = replayed
			m.setActivePage(current)
			return m, cmd, true
		}
	}
	if key == "g" {
		m.keyPrefix = "g"
		return m, nil, true
	}
	switch key {
	case "1":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabWorktrees})
		return next, cmd, true
	case "2":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabLog})
		return next, cmd, true
	case "3":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabStatus})
		return next, cmd, true
	case "4":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabStash})
		return next, cmd, true
	case "5":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabPRs})
		return next, cmd, true
	case "6":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabTickets})
		return next, cmd, true
	case "7":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabQueue})
		return next, cmd, true
	case "8":
		next, cmd := m.switchTab(nav.ViewState{Tab: nav.TabAgents})
		return next, cmd, true
	}
	return m, nil, false
}

func replayKeys(model tea.Model, msgs ...tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	current := model
	for _, msg := range msgs {
		next, cmd := current.Update(msg)
		current = next
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return current, tea.Batch(cmds...)
}

func (m Model) newLivePage(viewState nav.ViewState) (livePage, tea.Cmd) {
	entry, cmd := m.newHistoryEntry(viewState)
	return livePage{model: entry.model}, cmd
}

type tabSpec struct {
	id    nav.TabID
	label string
	group string
}

// tabSpecs is the single source of truth for view order, labels and groups,
// shared by the title row, the view switcher and the relative tab switch.
func (m Model) tabSpecs() []tabSpec {
	return []tabSpec{
		{id: nav.TabWorktrees, label: "worktrees", group: "git"},
		{id: nav.TabLog, label: "log", group: "git"},
		{id: nav.TabStatus, label: "status", group: "git"},
		{id: nav.TabStash, label: "stash", group: "git"},
		{id: nav.TabPRs, label: "prs", group: "git"},
		{id: nav.TabTickets, label: "tickets", group: "agents"},
		{id: nav.TabQueue, label: "queue", group: "agents"},
		{id: nav.TabAgents, label: "agents", group: "agents"},
	}
}

func (m Model) switchRelativeTab(delta int) (Model, tea.Cmd) {
	specs := m.tabSpecs()
	tabs := make([]nav.TabID, len(specs))
	for i, spec := range specs {
		tabs[i] = spec.id
	}
	idx := 0
	activeTab := m.navState.ActiveTab()
	for i, kind := range tabs {
		if kind == activeTab {
			idx = i
			break
		}
	}
	next := idx + delta
	if next < 0 {
		next = 0
	}
	if next >= len(tabs) {
		next = len(tabs) - 1
	}
	return m.switchTab(nav.ViewState{Tab: tabs[next]})
}
