package menu

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gx/ui"
	"gx/ui/components"

	tea "charm.land/bubbletea/v2"
)

// Item is a selectable menu entry.
type Item struct {
	Label  string
	Detail string // optional dim annotation shown after the label
}

// Run renders an interactive menu and returns the 0-based index of the chosen
// item, or -1 if the user aborted (esc/q/ctrl+c).
func Run(header string, items []Item) (int, error) {
	return run(header, items, os.Stdin, os.Stdout)
}

func run(header string, items []Item, in io.Reader, out io.Writer) (int, error) {
	m := model{header: header, items: items, state: menuState(items)}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))
	finalModel, err := p.Run()
	if err != nil {
		return -1, err
	}
	fm := finalModel.(model)
	if !fm.done || fm.aborted {
		return -1, nil
	}
	return fm.state.Cursor, nil
}

type model struct {
	header  string
	items   []Item
	state   components.MenuState
	done    bool
	aborted bool
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if state, decided, accepted, handled := components.UpdateMenu(msg, m.state); handled {
			m.state = state
			if decided {
				m.done = true
				m.aborted = !accepted
				return m, tea.Quit
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.aborted = true
			m.done = true
			return m, tea.Quit
		default:
			// Number shortcuts: 1-based
			key := msg.String()
			if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
				idx := int(key[0] - '1')
				if idx < len(m.items) {
					m.state.Cursor = idx
					m.done = true
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	var sb strings.Builder

	if m.header != "" {
		sb.WriteString(m.header)
		sb.WriteString("\n\n")
	}

	sb.WriteString(components.RenderMenuList(m.state, ui.ColorGray, ui.ColorGreen))
	sb.WriteString("\n")

	sb.WriteString("\n")
	sb.WriteString(ui.StyleDim.Render(components.MenuHintNumber))
	sb.WriteString("\n")

	return tea.NewView(sb.String())
}

func menuState(items []Item) components.MenuState {
	menuItems := make([]components.MenuItem, len(items))
	for i, item := range items {
		menuItems[i] = components.MenuItem{
			Label:  fmt.Sprintf("%d. %s", i+1, item.Label),
			Detail: item.Detail,
		}
	}
	return components.MenuState{Items: menuItems}
}
