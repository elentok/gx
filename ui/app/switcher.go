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

// switcherEnv is what the switcher reads from the shell: the tabs, the current
// one, the agents badge count and the screen size.
type switcherEnv struct {
	specs      []tabSpec
	current    nav.TabID
	actionable int
	width      int
	height     int
}

// switcherResult reports a pick; Picked is empty when nothing was picked.
type switcherResult struct{ Picked nav.TabID }

// openSwitcher builds the open "go to" list.
func openSwitcher(env switcherEnv) switcher {
	return switcher{isOpen: true, picker: components.PickerState{Items: env.items()}}
}

func (e switcherEnv) items() []components.PickerItem {
	var items []components.PickerItem
	for _, spec := range e.specs {
		item := components.PickerItem{Label: spec.label, Value: string(spec.id), Group: spec.group, Current: spec.id == e.current}
		if spec.id == nav.TabAgents && e.actionable > 0 {
			item.Badge = strconv.Itoa(e.actionable)
		}
		items = append(items, item)
	}
	return items
}

// sized is the picker sized to the rows left under the frame.
func (s switcher) sized(env switcherEnv) components.PickerState {
	p, _ := s.layout(env)
	return p
}

// layout sizes the picker to the rows left under the frame's top and wraps the
// hint to the frame. The hint, with the blank row the frame puts above it, is
// dropped when it would leave the list no row.
func (s switcher) layout(env switcherEnv) (components.PickerState, string) {
	p := s.picker
	rows := env.height - ui.ChromeRows - switcherChrome
	hint := lipgloss.NewStyle().Width(env.innerWidth()).Render(components.PickerHint)
	if hintRows := lipgloss.Height(hint) + 1; rows-hintRows >= 1 {
		rows -= hintRows
	} else {
		hint = ""
	}
	p.Height = max(rows, 1)
	return p, hint
}

func (s switcher) updateKey(key tea.KeyPressMsg, env switcherEnv) (switcher, tea.Cmd, switcherResult) {
	switch key.String() {
	case "ctrl+o":
		return switcher{}, nil, switcherResult{}
	case "ctrl+c":
		return switcher{}, nav.ForceQuit(), switcherResult{}
	}
	picker, decided, accepted, _ := components.UpdatePicker(key, s.sized(env))
	s.picker = picker
	if !decided {
		return s, nil, switcherResult{}
	}
	item, _ := picker.Selected()
	if !accepted {
		return switcher{}, nil, switcherResult{}
	}
	return switcher{}, nil, switcherResult{Picked: nav.TabID(item.Value)}
}

// updateMouse swallows every mouse message: a left click on a row picks it, a
// click outside the frame closes.
func (s switcher) updateMouse(msg tea.MouseMsg, env switcherEnv) (switcher, switcherResult) {
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return s, switcherResult{}
	}
	frame := s.render(env)
	w, h := lipgloss.Width(frame), lipgloss.Height(frame)
	x, y := click.X-switcherX, click.Y-ui.ChromeRows
	if x < 0 || x >= w || y < 0 || y >= h {
		return switcher{}, switcherResult{}
	}
	picker := s.sized(env)
	i, ok := picker.HitRow(y - switcherListTop)
	if !ok {
		return s, switcherResult{}
	}
	item := picker.Matches()[i].Item
	return switcher{}, switcherResult{Picked: nav.TabID(item.Value)}
}

func (e switcherEnv) outerWidth() int {
	if e.width > 0 {
		return min(switcherWidth, e.width-4)
	}
	return switcherWidth
}

// innerWidth leaves out two border columns and one padding column each side.
func (e switcherEnv) innerWidth() int {
	return max(e.outerWidth()-4, 1)
}

func (s switcher) render(env switcherEnv) string {
	picker, hint := s.layout(env)
	query := switcherPromptStyle.Render("›") + " " + picker.Filter
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "go to",
		Body:          query + "\n" + components.RenderPickerList(picker, env.innerWidth()),
		Hint:          hint,
		Width:         env.outerWidth(),
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		HintColor:     ui.ColorSubtle,
		TitleInBorder: true,
	})
}

// openSwitcher opens the "go to" list, under the same modal guard as switchTab.
func (m Model) openSwitcher() (Model, tea.Cmd) {
	type modalOpener interface{ ModalOpen() bool }
	if mo, ok := m.activePage().model.(modalOpener); ok && mo.ModalOpen() {
		return m, notify.Info("close the modal first")
	}
	m.keyPrefix = ""
	m.switcher = openSwitcher(m.switcherEnv())
	return m, nil
}

// applySwitcher stores the switcher's new state and switches to what it picked.
func (m Model) applySwitcher(s switcher, cmd tea.Cmd, res switcherResult) (Model, tea.Cmd) {
	m.switcher = s
	if res.Picked == "" {
		return m, cmd
	}
	return m.switchTab(nav.ViewState{Tab: res.Picked})
}

// switcherEnv gathers the switcher's inputs from the shell.
func (m Model) switcherEnv() switcherEnv {
	return switcherEnv{
		specs:      m.tabSpecs(),
		current:    m.activePage().viewState.Tab,
		actionable: m.stream.vm.ActionableCount(),
		width:      m.width,
		height:     m.height,
	}
}
