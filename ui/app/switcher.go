package app

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

// terminalFocuser is an optional page interface: while it reports true,
// ctrl+o belongs to the page's terminal, not to the view switcher.
type terminalFocuser interface{ TerminalFocused() bool }

const (
	switcherX     = 2
	switcherWidth = 34
	// switcherChrome is the frame's rows around the list: two borders and the
	// query line.
	switcherChrome = 3
)

var switcherPromptStyle = lipgloss.NewStyle().Foreground(ui.ColorBlue)

type switcher struct {
	isOpen bool
	picker components.PickerState
}

// openSwitcher opens the "go to" list, under the same modal guard as switchTab.
func (m Model) openSwitcher() (Model, tea.Cmd) {
	type modalOpener interface{ ModalOpen() bool }
	if mo, ok := m.activePage().model.(modalOpener); ok && mo.ModalOpen() {
		return m, notify.Info("close the modal first")
	}
	m.keyPrefix = ""
	m.switcher = switcher{isOpen: true, picker: components.PickerState{Items: m.switcherItems()}}
	return m, nil
}

func (m Model) switcherItems() []components.PickerItem {
	current := m.activePage().viewState.Tab
	var items []components.PickerItem
	for _, spec := range m.tabSpecs() {
		item := components.PickerItem{Label: spec.label, Value: string(spec.id), Group: spec.group, Current: spec.id == current}
		if spec.id == nav.TabAgents {
			if n := m.stream.vm.ActionableCount(); n > 0 {
				item.Badge = strconv.Itoa(n)
			}
		}
		items = append(items, item)
	}
	return items
}

// switcherPicker is the picker sized to the rows left under the frame's top.
func (m Model) switcherPicker() components.PickerState {
	p := m.switcher.picker
	p.Height = max(m.height-ui.ChromeRows-switcherChrome, 1)
	return p
}

func (m Model) updateSwitcherKey(key tea.KeyPressMsg) (Model, tea.Cmd) {
	if key.String() == "ctrl+o" {
		m.switcher = switcher{}
		return m, nil
	}
	picker, decided, accepted, _ := components.UpdatePicker(key, m.switcherPicker())
	m.switcher.picker = picker
	if !decided {
		return m, nil
	}
	item, _ := picker.Selected()
	m.switcher = switcher{}
	if !accepted {
		return m, nil
	}
	return m.switchTab(nav.ViewState{Tab: nav.TabID(item.Value)})
}

// updateSwitcherMouse swallows every mouse message: a left click on a row
// switches to it, a click outside the frame closes.
func (m Model) updateSwitcherMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return m, nil
	}
	frame := m.renderSwitcher()
	w, h := lipgloss.Width(frame), lipgloss.Height(frame)
	x, y := click.X-switcherX, click.Y-ui.ChromeRows
	if x < 0 || x >= w || y < 0 || y >= h {
		m.switcher = switcher{}
		return m, nil
	}
	// The list starts below the top border and the query line.
	picker := m.switcherPicker()
	i, ok := picker.HitRow(y - 2)
	if !ok {
		return m, nil
	}
	item := picker.Matches()[i].Item
	m.switcher = switcher{}
	return m.switchTab(nav.ViewState{Tab: nav.TabID(item.Value)})
}

func (m Model) switcherOuterWidth() int {
	if m.width > 0 {
		return min(switcherWidth, m.width-4)
	}
	return switcherWidth
}

func (m Model) renderSwitcher() string {
	picker := m.switcherPicker()
	// Two border columns and one padding column each side.
	innerW := max(m.switcherOuterWidth()-4, 1)
	query := switcherPromptStyle.Render("›") + " " + picker.Filter
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "go to",
		Body:          query + "\n" + components.RenderPickerList(picker, innerW),
		Width:         m.switcherOuterWidth(),
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		TitleInBorder: true,
	})
}
