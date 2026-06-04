package stashlist

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/ui"
	commitui "github.com/elentok/gx/ui/commit"
	"github.com/elentok/gx/ui/keys"
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
	}
	return t
}

// IsSplit reports whether the stash tab is in split mode (both panels visible).
func (t Tab) IsSplit() bool { return t.split.IsSplit() }

func (t Tab) Init() tea.Cmd {
	return t.stashList.Init()
}

func (t Tab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		var cmd tea.Cmd
		t.split, cmd = t.split.Update(msg)
		t = t.syncPanelSizes()
		return t, cmd
	}
	if key == "esc" && !t.split.IsCollapsed() {
		var cmd tea.Cmd
		t.split, cmd = t.split.Update(msg)
		t = t.syncPanelSizes()
		return t, cmd
	}
	if key == "f" && !t.split.IsCollapsed() {
		var cmd tea.Cmd
		t.split, cmd = t.split.Update(msg)
		t = t.syncPanelSizes()
		return t, cmd
	}
	if key == "t" {
		var cmd tea.Cmd
		t.split, cmd = t.split.Update(msg)
		t = t.syncPanelSizes()
		return t, cmd
	}

	if t.split.IsDetailFocused() {
		updated, cmd := t.commitDetail.Update(msg)
		t.commitDetail = updated.(commitui.Model)
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
		// Delegate enter to splitview to handle collapsed → split transition.
		var cmd tea.Cmd
		t.split, cmd = t.split.Update(msg)
		t = t.syncPanelSizes()
		return t, cmd
	}

	return t, nil
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

	lw, lh := t.split.ListSize()
	if lw > 0 && lh > 0 {
		updated, _ := t.stashList.Update(tea.WindowSizeMsg{Width: lw, Height: lh})
		t.stashList = updated.(Model)
	}
	listOut := t.stashList.View().Content

	if !t.split.IsSplit() {
		return ui.NewMainView(listOut)
	}

	detailContent := t.commitDetail.WithContainerFocus(t.split.IsDetailFocused()).View().Content
	var out string
	if t.split.EffectiveOrientation() == splitview.Vertical {
		out = lipgloss.JoinHorizontal(lipgloss.Top, listOut, detailContent)
	} else {
		out = lipgloss.JoinVertical(lipgloss.Left, listOut, detailContent)
	}
	return ui.NewMainView(out)
}
