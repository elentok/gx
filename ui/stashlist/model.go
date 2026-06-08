package stashlist

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/ui"
	commitui "github.com/elentok/gx/ui/commit"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/imagediff"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/splitview"
	stashpkg "github.com/elentok/gx/ui/stash"
)

// Model is the top-level stash tab model: a stash list paired with a commit
// detail panel in a split-view container.
type Model struct {
	worktreeRoot string
	settings     ui.Settings

	width  int
	height int

	stashList    listPanel
	split        splitview.Model
	commitDetail commitui.Model
	stashCreate  stashpkg.Model

	keys keys.Manager
	help help.Model
}

func NewModel(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) Model {
	list := newListPanel(worktreeRoot)
	detail := commitui.NewModel(worktreeRoot, "", "", settings, keys.Manager{})
	km := newStashManager()
	m := Model{
		worktreeRoot: worktreeRoot,
		settings:     settings,
		stashList:    list,
		commitDetail: detail,
		split:        splitview.NewSplit(list, detail),
		stashCreate:  stashpkg.New(),
		keys:         km,
		help:         help.NewModel(help.BuildSections(km, extraKeys)),
	}
	return m
}

// KeyManager exposes the stash tab's key bindings (used for chord overlays and
// help aggregation by the app shell).
func (m Model) KeyManager() keys.Manager {
	return m.keys
}

// IsSplit reports whether the stash tab is in split mode (both panels visible).
func (m Model) IsSplit() bool { return m.split.IsSplit() }

func (m Model) ModalOpen() bool {
	return m.stashCreate.IsOpen
}

func (m Model) Init() tea.Cmd {
	return m.stashList.Init()
}

func (m Model) Update(msg tea.Msg) (next tea.Model, cmd tea.Cmd) {
	// After every Update, re-sync the detail panel's injected screen origin so
	// its image-diff overlay tracks layout/visibility/modal changes (ADR 0010).
	// WithScreenOrigin no-ops unless the origin or visibility actually changed.
	defer func() {
		model, ok := next.(Model)
		if !ok {
			return
		}
		var originCmd tea.Cmd
		model, originCmd = model.withSyncedDetailOrigin()
		if originCmd != nil {
			cmd = tea.Batch(cmd, originCmd)
			next = model
		}
	}()

	if m.stashCreate.IsOpen {
		return m.handleStashCreateUpdate(msg)
	}

	if m.help.IsOpen {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		var cmd tea.Cmd
		m.split, cmd = m.split.Update(msg)
		m.help, _ = m.help.Update(msg)
		m = m.syncPanelSizes()
		return m, cmd

	case stashLoadedMsg:
		updated, cmd := m.stashList.Update(msg)
		m.stashList = updated.(listPanel)
		m.split = m.split.WithListRef(m.stashList.SelectedRef())
		// Load the first entry in the commit detail.
		if ref := m.stashList.SelectedRef(); ref != "" {
			var refCmd tea.Cmd
			m.commitDetail, refCmd = m.commitDetail.WithRef(ref)
			m = m.syncPanelSizes()
			return m, tea.Batch(cmd, refCmd)
		}
		return m, cmd

	case stashAutoReloadMsg:
		return m, m.stashList.cmdLoad()

	case stashApplyDoneMsg:
		return m.handleApplyDone(msg)

	case stashPopDoneMsg:
		return m.handlePopDone(msg)

	case stashDropDoneMsg:
		return m.handleDropDone(msg)

	case splitview.SelectionChangedMsg:
		if msg.Ref != "" {
			var refCmd tea.Cmd
			m.commitDetail, refCmd = m.commitDetail.WithRef(msg.Ref)
			m = m.syncPanelSizes()
			return m, refCmd
		}
		return m, nil

	case imagediff.SettleMsg:
		// commit.Model's only async round-trip; forward the debounce tick to the
		// detail panel that owns the overlay (ADR 0010).
		updated, detailCmd := m.commitDetail.Update(msg)
		m.commitDetail = updated.(commitui.Model)
		return m, detailCmd

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Broadcast non-key messages to detail panel.
	updated, detailCmd := m.commitDetail.Update(msg)
	m.commitDetail = updated.(commitui.Model)
	return m, detailCmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Chord from splitview takes priority.
	if m.split.HasChord() {
		return m.routeKeyToSplit(msg)
	}
	if key == "h" && m.split.IsSplit() && m.split.IsDetailFocused() && (m.commitDetail.IsFileTreeFocused() || m.commitDetail.IsHeaderFocused()) {
		return m.routeKeyToSplit(tea.KeyPressMsg{Code: tea.KeyEsc})
	}
	if key == "q" {
		if m.split.IsDetailFocused() && m.commitDetail.HasInternalFocus() {
			updated, cmd := m.commitDetail.Update(msg)
			m.commitDetail = updated.(commitui.Model)
			return m, cmd
		}
		if m.split.IsDetailFocused() {
			return m.routeKeyToSplit(msg)
		}
		return m, nav.Back()
	}
	if key == "esc" && !m.split.IsCollapsed() {
		if m.split.IsDetailFocused() && m.commitDetail.HasInternalFocus() {
			updated, cmd := m.commitDetail.Update(msg)
			m.commitDetail = updated.(commitui.Model)
			return m, cmd
		}
		return m.routeKeyToSplit(msg)
	}
	if key == "f" && !m.split.IsCollapsed() {
		return m.routeKeyToSplit(msg)
	}
	if key == "t" {
		return m.routeKeyToSplit(msg)
	}

	if m.split.IsDetailFocused() {
		updated, cmd := m.commitDetail.Update(msg)
		m.commitDetail = updated.(commitui.Model)
		return m, cmd
	}

	// List panel active: dispatch through the key manager.
	if match, consumed := m.keys.Process(msg); match != nil {
		return m.dispatchBinding(match.ID, msg)
	} else if consumed {
		return m, nil
	}

	return m, nil
}

func (m Model) routeKeyToSplit(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.split, cmd = m.split.Update(msg)
	m = m.syncPanelSizes()
	return m, cmd
}

// withSyncedDetailOrigin pushes the detail panel's absolute screen origin (and
// visibility) into commitDetail so its image-diff kitty overlay lands where the
// panel is composed (ADR 0010). The detail is treated as not visible whenever a
// stash-tab modal is open, since a centered modal occludes it.
func (m Model) withSyncedDetailOrigin() (Model, tea.Cmd) {
	col, row, visible := m.split.DetailOrigin()
	visible = visible && !m.stashCreate.IsOpen && !m.help.IsOpen
	var cmd tea.Cmd
	m.commitDetail, cmd = m.commitDetail.WithScreenOrigin(col, row, visible)
	return m, cmd
}

// OnPageDeactivated is called by the app shell when the user switches away from
// the stash tab. It clears any active image-diff overlay in the detail panel so
// it doesn't float over the next tab (ADR 0010).
func (m Model) OnPageDeactivated() tea.Cmd {
	return m.commitDetail.OnDeactivate()
}

func (m Model) syncPanelSizes() Model {
	lw, lh := m.split.ListSize()
	if lw > 0 && lh > 0 {
		updated, _ := m.stashList.Update(tea.WindowSizeMsg{Width: lw, Height: lh})
		m.stashList = updated.(listPanel)
	}
	dw, dh := m.split.DetailSize()
	if dw > 0 && dh > 0 {
		updated, _ := m.commitDetail.Update(tea.WindowSizeMsg{Width: dw, Height: dh})
		m.commitDetail = updated.(commitui.Model)
	}
	return m
}

func (m Model) View() tea.View {
	if m.split.IsFullscreen() && m.split.IsDetailFocused() {
		return m.commitDetail.WithContainerFocus(true).View()
	}

	out := m.buildMainContent()
	if m.stashCreate.IsOpen {
		out = ui.OverlayCenter(out, m.stashCreate.View(m.width), m.width, m.height)
	}
	if m.help.IsOpen {
		out = ui.OverlayCenter(out, m.help.View(), m.width, m.height)
	}
	return ui.NewMainView(out)
}

func (m Model) buildMainContent() string {
	lw, lh := m.split.ListSize()
	if lw > 0 && lh > 0 {
		updated, _ := m.stashList.Update(tea.WindowSizeMsg{Width: lw, Height: lh})
		m.stashList = updated.(listPanel)
	}
	listOut := m.stashList.WithContainerFocus(m.isListActive()).View().Content

	if !m.split.IsSplit() {
		return lipgloss.JoinVertical(lipgloss.Left, listOut, stashFooter())
	}

	detailContent := m.commitDetail.WithContainerFocus(m.split.IsDetailFocused()).View().Content
	var body string
	if m.split.EffectiveOrientation() == splitview.Vertical {
		body = lipgloss.JoinHorizontal(lipgloss.Top, listOut, detailContent)
	} else {
		body = lipgloss.JoinVertical(lipgloss.Left, listOut, detailContent)
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, stashFooter())
}

func (m Model) isListActive() bool {
	return !m.split.IsSplit() || m.split.IsListFocused()
}

func stashFooter() string {
	return "  " + ui.StyleHint.Render("? help")
}
