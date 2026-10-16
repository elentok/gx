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
	// switcherListTop is the frame's rows above the list: the top border and
	// the query line.
	switcherListTop = 2
	// switcherChrome is the frame's rows around the list, before the hint.
	switcherChrome = switcherListTop + 1
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

func (m Model) switcherPicker() components.PickerState {
	p, _ := m.switcherLayout()
	return p
}

// switcherLayout sizes the picker to the rows left under the frame's top and
// wraps the hint to the frame. The hint, with the blank row the frame puts
// above it, is dropped when it would leave the list no row.
func (m Model) switcherLayout() (components.PickerState, string) {
	p := m.switcher.picker
	rows := m.height - ui.ChromeRows - switcherChrome
	hint := lipgloss.NewStyle().Width(m.switcherInnerWidth()).Render(components.PickerHint)
	if hintRows := lipgloss.Height(hint) + 1; rows-hintRows >= 1 {
		rows -= hintRows
	} else {
		hint = ""
	}
	p.Height = max(rows, 1)
	return p, hint
}

func (m Model) updateSwitcherKey(key tea.KeyPressMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+o":
		m.switcher = switcher{}
		return m, nil
	case "ctrl+c":
		m.switcher = switcher{}
		return m, nav.ForceQuit()
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
	picker := m.switcherPicker()
	i, ok := picker.HitRow(y - switcherListTop)
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

// switcherInnerWidth leaves out two border columns and one padding column
// each side.
func (m Model) switcherInnerWidth() int {
	return max(m.switcherOuterWidth()-4, 1)
}

func (m Model) renderSwitcher() string {
	picker, hint := m.switcherLayout()
	query := switcherPromptStyle.Render("›") + " " + picker.Filter
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "go to",
		Body:          query + "\n" + components.RenderPickerList(picker, m.switcherInnerWidth()),
		Hint:          hint,
		Width:         m.switcherOuterWidth(),
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		HintColor:     ui.ColorSubtle,
		TitleInBorder: true,
	})
}
