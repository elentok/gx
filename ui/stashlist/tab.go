package stashlist

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/ui"
	commitui "github.com/elentok/gx/ui/commit"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	stashpkg "github.com/elentok/gx/ui/stash"
	"github.com/elentok/gx/ui/splitview"
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
}

func NewTab(worktreeRoot string, settings ui.Settings, extraKeys keys.Manager) Tab {
	list := NewModel(worktreeRoot)
	detail := commitui.NewModel(worktreeRoot, "", "", settings, keys.Manager{})
	t := Tab{
		worktreeRoot: worktreeRoot,
		settings:     settings,
		stashList:    list,
		commitDetail: detail,
		split:        splitview.NewSplit(list, detail),
		stashCreate:  stashpkg.New(),
	}
	return t
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

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		var cmd tea.Cmd
		t.split, cmd = t.split.Update(msg)
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

	// Stash operations — only active when the list panel has focus.
	switch key {
	case "a":
		if ref := t.stashList.SelectedRef(); ref != "" {
			return t, t.cmdApply(ref)
		}
	case "p":
		if ref := t.stashList.SelectedRef(); ref != "" {
			return t, t.cmdPopRef(ref)
		}
	case "d":
		if ref := t.stashList.SelectedRef(); ref != "" {
			return t, t.cmdDrop(ref)
		}
	case "s":
		cmd := t.stashCreate.Open(t.worktreeRoot, false)
		return t, cmd
	}

	// Route navigation keys to the stash list.
	prevRef := t.stashList.SelectedRef()
	switch key {
	case "j", "down", "k", "up", "G", "shift+g":
		updated, cmd := t.stashList.Update(msg)
		t.stashList = updated.(Model)
		t.split = t.split.WithListRef(t.stashList.SelectedRef())
		if ref := t.stashList.SelectedRef(); t.split.IsSplit() && ref != prevRef && ref != "" {
			t.commitDetail = t.commitDetail.WithRef(ref)
			t = t.syncPanelSizes()
		}
		return t, cmd

	case "enter":
		return t.routeKeyToSplit(msg)
	case "l":
		return t.routeKeyToSplit(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	return " "
}
