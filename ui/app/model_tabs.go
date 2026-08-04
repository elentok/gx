package app

import (
	"strings"
	"unicode"

	"github.com/elentok/gx/ui"
	logui "github.com/elentok/gx/ui/log"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/navstate"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) ensureLivePages() {
	for _, kind := range []nav.TabID{nav.TabWorktrees, nav.TabLog, nav.TabStatus, nav.TabStash, nav.TabPRs, nav.TabTickets, nav.TabQueue} {
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

func (m Model) applySwitch(tabVS, prevVS nav.ViewState) (Model, tea.Cmd) {
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
		currentPage = m.newLivePage(tabVS)
		currentPage.didInit = true
		currentPage.viewState = tabVS
		m.livePageByTab[tabVS.Tab] = currentPage
		// Init loads fresh data; stamp the tab so it won't auto-reload on the next switch.
		m.gate.MarkLoaded(tabVS.Tab)
		return m, tea.Batch(onPageDeactivatedCmd(outgoing), currentPage.model.Init(), m.resizeCurrentCmd(), onPageActivatedCmd(currentPage.model))
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

func injectTabsIntoFooter(content, tabs string, width int) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}
	if width <= 0 {
		lines[len(lines)-1] = lines[len(lines)-1] + " " + tabs
		return strings.Join(lines, "\n")
	}
	tabs = ansi.Truncate(tabs, width, "")
	tabsW := ansi.StringWidth(tabs)
	if tabsW >= width {
		lines[len(lines)-1] = tabs
		return strings.Join(lines, "\n")
	}
	rightMax := width - tabsW - 1
	if rightMax < 0 {
		rightMax = 0
	}
	rightContent := strings.TrimLeftFunc(lines[len(lines)-1], unicode.IsSpace)
	right := rightContent
	rightW := ansi.StringWidth(rightContent)
	if rightW > rightMax {
		if rightMax <= 0 {
			right = ""
		} else if rightMax == 1 {
			right = "…"
		} else {
			// Keep the tail where compact footer hints live (context/mode/help).
			right = "…" + ansi.TruncateLeft(rightContent, rightW-rightMax+1, "")
		}
	}
	rightW = ansi.StringWidth(right)
	gap := width - tabsW - rightW
	if gap < 1 {
		gap = 1
	}
	lines[len(lines)-1] = tabs + strings.Repeat(" ", gap) + right
	return strings.Join(lines, "\n")
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

func (m Model) newLivePage(viewState nav.ViewState) livePage {
	return livePage{
		model: m.newHistoryEntry(viewState).model,
	}
}

func (m Model) tabsView() string {
	specs := m.tabSpecs()
	parts := make([]string, 0, len(specs))
	for _, tab := range specs {
		parts = append(parts, renderTab(tab))
	}

	return strings.Join(parts, " ")
}

type tabSpec struct {
	id     nav.TabID
	label  string
	active bool
}

// tabSpecs is the single source of truth for tab order/labels, shared by
// tabsView (rendering) and tabHitAt (click hit-testing) so they can't drift.
func (m Model) tabSpecs() []tabSpec {
	activeTab := m.navState.ActiveTab()
	return []tabSpec{
		{id: nav.TabWorktrees, label: "worktrees", active: activeTab == nav.TabWorktrees},
		{id: nav.TabLog, label: "log", active: activeTab == nav.TabLog},
		{id: nav.TabStatus, label: "status", active: activeTab == nav.TabStatus},
		{id: nav.TabStash, label: "stash", active: activeTab == nav.TabStash},
		{id: nav.TabPRs, label: "prs", active: activeTab == nav.TabPRs},
		{id: nav.TabTickets, label: "tickets", active: activeTab == nav.TabTickets},
		{id: nav.TabQueue, label: "queue", active: activeTab == nav.TabQueue},
	}
}

func renderTab(tab tabSpec) string {
	if tab.active {
		return ui.RenderBadge(tab.label, ui.BadgeVariantOrange, true, false)
	}
	return ui.RenderBadge(tab.label, ui.BadgeVariantSurface, true, false)
}

// tabHitAt reports which tab (if any) occupies screen coordinate (x, y),
// recomputing the same left-to-right layout injectTabsIntoFooter renders
// every frame — tabs always start at column 0 of the footer's last line, so
// no state needs to survive between render and click. A tab whose full label
// is truncated off-screen at the current width is not reported as a hit.
func (m Model) tabHitAt(x, y int) (nav.TabID, bool) {
	if m.width <= 0 || y != m.height-1 {
		return "", false
	}
	pos := 0
	for _, spec := range m.tabSpecs() {
		w := ansi.StringWidth(renderTab(spec))
		end := pos + w
		if end > m.width {
			return "", false
		}
		if x >= pos && x < end {
			return spec.id, true
		}
		pos = end + 1 // account for the single-space separator
	}
	return "", false
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
