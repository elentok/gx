package confirm

import (
	"io"
	"os"
	"strings"

	"gx/ui"

	tea "charm.land/bubbletea/v2"
)

type doneMsg struct{}

type model struct {
	prompt    string
	choiceYes bool
	done      bool
	nerd      bool
}

// Run renders a small styled confirmation UI and returns true when accepted.
// Yes is the default selection.
func Run(prompt string) (bool, error) {
	return run(prompt, false, os.Stdin, os.Stdout)
}

// RunWithNerd is like Run but uses nerd-font pill-shaped buttons when nerd is true.
func RunWithNerd(prompt string, nerd bool) (bool, error) {
	return run(prompt, nerd, os.Stdin, os.Stdout)
}

func run(prompt string, nerd bool, in io.Reader, out io.Writer) (bool, error) {
	m := model{prompt: prompt, choiceYes: true, nerd: nerd}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))
	finalModel, err := p.Run()
	if err != nil {
		return false, err
	}
	fm := finalModel.(model)
	return fm.done && fm.choiceYes, nil
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "left", "h":
			m.choiceYes = true
		case "right", "l":
			m.choiceYes = false
		case "y":
			m.choiceYes = true
			m.done = true
			return m, tea.Quit
		case "n":
			m.choiceYes = false
			m.done = true
			return m, tea.Quit
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc", "q":
			m.choiceYes = false
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	hint := ui.StyleDim.Render("left/right: choose  y/n: quick select  enter: confirm")
	yes := ui.RenderButton("Yes", m.choiceYes, m.nerd)
	no := ui.RenderButton("No", !m.choiceYes, m.nerd)
	return tea.NewView(strings.Join([]string{
		m.prompt,
		"",
		"  " + yes + "   " + no,
		"  " + hint,
		"",
	}, "\n"))
}
