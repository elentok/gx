package stashlist

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/ui"
	commitui "github.com/elentok/gx/ui/commit"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/splitview"
	stashpkg "github.com/elentok/gx/ui/stash"
)

// Tab is the top-level stash tab model: a stash list paired with a commit
// detail panel in a split-view container.
type Tab struct {
	worktreeRoot string
	settings     ui.Settings

	width  int
	height int

	stashList    Model
	split        splitview.Model
	commitDetail commitui.Model
	stashCreate  stashpkg.Model

	keys keys.Manager
	help help.Model
}

func NewTab(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) Tab {
	list := NewModel(worktreeRoot)
	detail := commitui.NewModel(worktreeRoot, "", "", settings, keys.Manager{})
	km := newStashManager()
	t := Tab{
		worktreeRoot: worktreeRoot,
		settings:     settings,
		stashList:    list,
		commitDetail: detail,
		split:        splitview.NewSplit(list, detail),
		stashCreate:  stashpkg.New(),
		keys:         km,
		help:         help.NewModel(help.BuildSections(km, extraKeys)),
	}
	return t
}

// KeyManager exposes the stash tab's key bindings (used for chord overlays and
// help aggregation by the app shell).
func (t Tab) KeyManager() keys.Manager {
	return t.keys
}

// IsSplit reports whether the stash tab is in split mode (both panels visible).
func (t Tab) IsSplit() bool { return t.split.IsSplit() }

func (t Tab) Init() tea.Cmd {
	return t.stashList.Init()
}

func (t Tab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if t.stashCreate.IsOpen {
		return t.handleStashCreateUpdate(msg)
	}

	if t.help.IsOpen {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			var cmd tea.Cmd
			t.help, cmd = t.help.Update(msg)
			return t, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		var cmd tea.Cmd
		t.split, cmd = t.split.Update(msg)
		t.help, _ = t.help.Update(msg)
		t = t.syncPanelSizes()
		return t, cmd

	case stashLoadedMsg:
		updated, cmd := t.stashList.Update(msg)
		t.stashList = updated.(Model)
		t.split = t.split.WithListRef(t.stashList.SelectedRef())
		// Load the first entry in the commit detail.
		if ref := t.stashList.SelectedRef(); ref != "" {
			t.commitDetail = t.commitDetail.WithRef(ref)
			t = t.syncPanelSizes()
		}
		return t, cmd

	case stashAutoReloadMsg:
		return t, t.stashList.cmdLoad()

	case stashApplyDoneMsg:
		return t.handleApplyDone(msg)

	case stashPopDoneMsg:
		return t.handlePopDone(msg)

	case stashDropDoneMsg:
		return t.handleDropDone(msg)

	case splitview.SelectionChangedMsg:
		if msg.Ref != "" {
			t.commitDetail = t.commitDetail.WithRef(msg.Ref)
			t = t.syncPanelSizes()
		}
		return t, nil

	case tea.KeyPressMsg:
		return t.handleKey(msg)
	}

	// Broadcast non-key messages to detail panel.
	var cmd tea.Cmd
	updated, cmd := t.commitDetail.Update(msg)
	t.commitDetail = updated.(commitui.Model)
	return t, cmd
}

func (t Tab) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Chord from splitview takes priority.
	if t.split.HasChord() {
		return t.routeKeyToSplit(msg)
	}
	if key == "h" && t.split.IsSplit() && t.split.IsDetailFocused() && (t.commitDetail.IsFileTreeFocused() || t.commitDetail.IsHeaderFocused()) {
		return t.routeKeyToSplit(tea.KeyPressMsg{Code: tea.KeyEsc})
	}
	if key == "q" {
		if t.split.IsDetailFocused() && t.commitDetail.HasInternalFocus() {
			updated, cmd := t.commitDetail.Update(msg)
			t.commitDetail = updated.(commitui.Model)
			return t, cmd
		}
		if t.split.IsDetailFocused() {
			return t.routeKeyToSplit(msg)
		}
		return t, nav.Back()
	}
	if key == "esc" && !t.split.IsCollapsed() {
		if t.split.IsDetailFocused() && t.commitDetail.HasInternalFocus() {
			updated, cmd := t.commitDetail.Update(msg)
			t.commitDetail = updated.(commitui.Model)
			return t, cmd
		}
		return t.routeKeyToSplit(msg)
	}
	if key == "f" && !t.split.IsCollapsed() {
		return t.routeKeyToSplit(msg)
	}
	if key == "t" {
		return t.routeKeyToSplit(msg)
	}

	if t.split.IsDetailFocused() {
		updated, cmd := t.commitDetail.Update(msg)
		t.commitDetail = updated.(commitui.Model)
		return t, cmd
	}

	// List panel active: dispatch through the key manager.
	if match, consumed := t.keys.Process(msg); match != nil {
		return t.dispatchBinding(match.ID, msg)
	} else if consumed {
		return t, nil
	}

	return t, nil
}

func (t Tab) routeKeyToSplit(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	t.split, cmd = t.split.Update(msg)
	t = t.syncPanelSizes()
	return t, cmd
}

func (t Tab) syncPanelSizes() Tab {
	lw, lh := t.split.ListSize()
	if lw > 0 && lh > 0 {
		updated, _ := t.stashList.Update(tea.WindowSizeMsg{Width: lw, Height: lh})
		t.stashList = updated.(Model)
	}
	dw, dh := t.split.DetailSize()
	if dw > 0 && dh > 0 {
		updated, _ := t.commitDetail.Update(tea.WindowSizeMsg{Width: dw, Height: dh})
		t.commitDetail = updated.(commitui.Model)
	}
	return t
}

func (t Tab) View() tea.View {
	if t.split.IsFullscreen() && t.split.IsDetailFocused() {
		return t.commitDetail.WithContainerFocus(true).View()
	}

	out := t.buildMainContent()
	if t.stashCreate.IsOpen {
		out = ui.OverlayCenter(out, t.stashCreate.View(t.width), t.width, t.height)
	}
	if t.help.IsOpen {
		out = ui.OverlayCenter(out, t.help.View(), t.width, t.height)
	}
	return ui.NewMainView(out)
}

func (t Tab) buildMainContent() string {
	lw, lh := t.split.ListSize()
	if lw > 0 && lh > 0 {
		updated, _ := t.stashList.Update(tea.WindowSizeMsg{Width: lw, Height: lh})
		t.stashList = updated.(Model)
	}
	listOut := t.stashList.WithContainerFocus(t.isListActive()).View().Content

	if !t.split.IsSplit() {
		return lipgloss.JoinVertical(lipgloss.Left, listOut, stashFooter())
	}

	detailContent := t.commitDetail.WithContainerFocus(t.split.IsDetailFocused()).View().Content
	var body string
	if t.split.EffectiveOrientation() == splitview.Vertical {
		body = lipgloss.JoinHorizontal(lipgloss.Top, listOut, detailContent)
	} else {
		body = lipgloss.JoinVertical(lipgloss.Left, listOut, detailContent)
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, stashFooter())
}

func (t Tab) isListActive() bool {
	return !t.split.IsSplit() || t.split.IsListFocused()
}

func stashFooter() string {
	return "  " + ui.StyleHint.Render("? help")
}
