package stashlist

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/list"
)

type stashLoadedMsg struct {
	entries []git.StashEntry
	err     error
}

// listPanel is the stash list panel. It implements splitview.ListPanel.
type listPanel struct {
	worktreeRoot string
	entries      []git.StashEntry
	list         list.Model
	width        int
	height       int
	loaded       bool
	err          error
	inactive     bool
}

func newListPanel(worktreeRoot string) listPanel {
	return listPanel{worktreeRoot: worktreeRoot}
}

// WithContainerFocus returns a copy rendered as active only when its
// containing split/list panel owns keyboard focus.
func (m listPanel) WithContainerFocus(focused bool) listPanel {
	m.inactive = !focused
	return m
}

func (m listPanel) Init() tea.Cmd {
	return m.cmdLoad()
}

// SelectedRef returns the git ref of the currently selected stash entry.
func (m listPanel) SelectedRef() string {
	if len(m.entries) == 0 {
		return ""
	}
	sel := m.list.Selected()
	if sel >= len(m.entries) {
		return ""
	}
	return m.entries[sel].Ref
}

func (m listPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case stashLoadedMsg:
		m.loaded = true
		m.err = msg.err
		m.entries = msg.entries
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "j", "down":
			m.list.Navigate(1, len(m.entries), m.visibleH())
		case "k", "up":
			m.list.Navigate(-1, len(m.entries), m.visibleH())
		case "G", "shift+g":
			m.list.SetSelected(len(m.entries)-1, len(m.entries))
			m.list.EnsureSelectionVisible(len(m.entries), m.visibleH())
		case "g":
			// "gg" handled by splitview or parent; single g is ignored here
		}
	}
	return m, nil
}

func (m listPanel) visibleH() int {
	h := m.height - 3 // frame is height-1, minus top+bottom borders
	if h < 1 {
		return 1
	}
	return h
}

func (m listPanel) cmdLoad() tea.Cmd {
	root := m.worktreeRoot
	return func() tea.Msg {
		entries, err := git.StashList(root)
		return stashLoadedMsg{entries: entries, err: err}
	}
}
