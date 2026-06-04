package splitview

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Orientation controls the layout direction.
type Orientation int

const (
	// Vertical places the list on the left and detail on the right.
	Vertical Orientation = iota
	// Horizontal places the list on top and detail on bottom.
	Horizontal
)

type visMode int

const (
	visModeCollapsed visMode = iota
	visModeSplit
	visModeFullscreen
)

type focusTarget int

const (
	focusList focusTarget = iota
	focusDetail
)

// ListPanel is the interface the list side of the split view must implement.
type ListPanel interface {
	tea.Model
	SelectedRef() string
}

// DetailPanel is the interface the detail side of the split view must implement.
type DetailPanel interface {
	tea.Model
}

// SelectionChangedMsg is fired when the list selection changes while in Split state.
type SelectionChangedMsg struct {
	Ref string
}

type savedState struct {
	vis   visMode
	focus focusTarget
}

// Model is the split-view container that manages panel visibility, focus, and
// key routing for a list+detail layout used by the log and stash tabs.
type Model struct {
	list   ListPanel
	detail DetailPanel

	vis   visMode
	focus focusTarget
	prev  *savedState // non-nil while in fullscreen, holds state to restore

	autoOrient  bool
	orientation Orientation

	width  int
	height int

	keyPrefix string
}

// New creates a Model with auto-orientation enabled (width ≤100 → Horizontal).
func New(list ListPanel, detail DetailPanel) Model {
	return Model{
		list:       list,
		detail:     detail,
		vis:        visModeCollapsed,
		focus:      focusList,
		autoOrient: true,
	}
}

// NewSplit creates a Model that starts in split mode (both panels visible).
func NewSplit(list ListPanel, detail DetailPanel) Model {
	m := New(list, detail)
	m.vis = visModeSplit
	return m
}

// IsCollapsed reports whether only the list panel is visible.
func (m Model) IsCollapsed() bool { return m.vis == visModeCollapsed }

// IsSplit reports whether both panels are visible.
func (m Model) IsSplit() bool { return m.vis == visModeSplit }

// IsFullscreen reports whether a panel is fullscreened.
func (m Model) IsFullscreen() bool { return m.vis == visModeFullscreen }

// IsListFocused reports whether the list panel has keyboard focus.
func (m Model) IsListFocused() bool { return m.focus == focusList }

// IsDetailFocused reports whether the detail panel has keyboard focus.
func (m Model) IsDetailFocused() bool { return m.focus == focusDetail }

// HasChord reports whether the split view is waiting for the second key of a
// multi-key shortcut (e.g. the "t" in "to"). The containing model should
// route the next key to the split view when this returns true.
func (m Model) HasChord() bool { return m.keyPrefix != "" }

// EffectiveOrientation returns the resolved layout direction.
func (m Model) EffectiveOrientation() Orientation { return m.effectiveOrientation() }

// WithListRef updates the ref reported by the list panel adapter. Used by
// containers that manage their own list rendering (such as ui/log) so the
// split container can detect selection changes for auto-update.
func (m Model) WithListRef(ref string) Model {
	m.list = logListRef{ref: ref, inner: m.list}
	return m
}

// logListRef wraps any ListPanel and overrides its SelectedRef return value.
type logListRef struct {
	ref   string
	inner ListPanel
}

func (l logListRef) Init() tea.Cmd                           { return l.inner.Init() }
func (l logListRef) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return l, nil }
func (l logListRef) View() tea.View                          { return l.inner.View() }
func (l logListRef) SelectedRef() string                     { return l.ref }

// ListSize returns the (width, height) the list panel should render at.
func (m Model) ListSize() (w, h int) {
	switch m.vis {
	case visModeFullscreen:
		if m.focus == focusDetail {
			return 0, 0
		}
		return m.width, m.height
	case visModeCollapsed:
		return m.width, m.height
	default: // visModeSplit
		return m.splitListDims()
	}
}

// DetailSize returns the (width, height) the detail panel should render at.
func (m Model) DetailSize() (w, h int) {
	switch m.vis {
	case visModeFullscreen:
		if m.focus == focusList {
			return 0, 0
		}
		return m.width, m.height
	case visModeCollapsed:
		return 0, 0
	default: // visModeSplit
		lw, lh := m.splitListDims()
		if m.effectiveOrientation() == Vertical {
			return m.width - lw, m.height
		}
		return m.width, m.height - lh
	}
}

// effectiveOrientation returns the resolved layout direction.
func (m Model) effectiveOrientation() Orientation {
	if m.autoOrient {
		if m.width <= 100 {
			return Horizontal
		}
		return Vertical
	}
	return m.orientation
}

func (m Model) splitListDims() (w, h int) {
	if m.effectiveOrientation() == Vertical {
		return max(1, int(float64(m.width)*0.40)), m.height
	}
	return m.width, max(1, int(float64(m.height)*0.30))
}

// Init initializes both panels.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.list.Init(), m.detail.Init())
}

// Update processes a message and returns the updated model and any commands.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.resize()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Broadcast non-key messages to both panels.
	var cmds []tea.Cmd
	var cmd tea.Cmd

	prevRef := m.list.SelectedRef()
	m.list, cmd = updateList(m.list, msg)
	cmds = append(cmds, cmd)

	if m.vis == visModeSplit {
		if ref := m.list.SelectedRef(); ref != prevRef {
			r := ref
			cmds = append(cmds, func() tea.Msg { return SelectionChangedMsg{Ref: r} })
		}
	}

	m.detail, cmd = updateDetail(m.detail, msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	key := msg.String()

	// Resolve "to" chord for orientation toggle.
	if m.keyPrefix == "t" {
		m.keyPrefix = ""
		if key == "o" {
			m.autoOrient = false
			if m.orientation == Vertical {
				m.orientation = Horizontal
			} else {
				m.orientation = Vertical
			}
			return m.resize()
		}
		return m.delegateKey(msg)
	}
	if key == "t" {
		m.keyPrefix = "t"
		return m, nil
	}

	switch key {
	case "enter":
		if m.focus == focusList {
			switch m.vis {
			case visModeCollapsed:
				m.vis = visModeSplit
				m.focus = focusDetail
				return m.resize()
			case visModeSplit:
				m.focus = focusDetail
				return m, nil
			}
		}

	case "esc", "q":
		if m.vis == visModeSplit {
			switch m.focus {
			case focusDetail:
				m.focus = focusList
				return m, nil
			case focusList:
				m.vis = visModeCollapsed
				return m.resize()
			}
		}

	case "f":
		if m.vis == visModeFullscreen {
			if m.prev != nil {
				m.vis = m.prev.vis
				m.focus = m.prev.focus
				m.prev = nil
			}
			return m.resize()
		}
		m.prev = &savedState{vis: m.vis, focus: m.focus}
		m.vis = visModeFullscreen
		return m.resize()
	}

	return m.delegateKey(msg)
}

func (m Model) delegateKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.focus == focusList {
		prevRef := m.list.SelectedRef()
		m.list, cmd = updateList(m.list, msg)
		if m.vis == visModeSplit {
			if ref := m.list.SelectedRef(); ref != prevRef {
				r := ref
				selCmd := func() tea.Msg { return SelectionChangedMsg{Ref: r} }
				return m, tea.Batch(cmd, selCmd)
			}
		}
	} else {
		m.detail, cmd = updateDetail(m.detail, msg)
	}
	return m, cmd
}

// View renders the panels according to the current visibility state and orientation.
func (m Model) View() tea.View {
	listContent := m.list.View().Content
	detailContent := m.detail.View().Content

	switch m.vis {
	case visModeCollapsed:
		return tea.NewView(listContent)
	case visModeFullscreen:
		if m.focus == focusList {
			return tea.NewView(listContent)
		}
		return tea.NewView(detailContent)
	}

	// visModeSplit
	if m.effectiveOrientation() == Vertical {
		return tea.NewView(lipgloss.JoinHorizontal(lipgloss.Top, listContent, detailContent))
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, listContent, detailContent))
}

// resize sends current panel dimensions to both sub-panels via their Update methods.
func (m Model) resize() (Model, tea.Cmd) {
	lw, lh := m.ListSize()
	dw, dh := m.DetailSize()
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.list, cmd = updateList(m.list, tea.WindowSizeMsg{Width: lw, Height: lh})
	cmds = append(cmds, cmd)
	m.detail, cmd = updateDetail(m.detail, tea.WindowSizeMsg{Width: dw, Height: dh})
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func updateList(l ListPanel, msg tea.Msg) (ListPanel, tea.Cmd) {
	updated, cmd := l.Update(msg)
	return updated.(ListPanel), cmd
}

func updateDetail(d DetailPanel, msg tea.Msg) (DetailPanel, tea.Cmd) {
	updated, cmd := d.Update(msg)
	return updated.(DetailPanel), cmd
}
