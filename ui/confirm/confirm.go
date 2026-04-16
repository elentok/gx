package confirm

import (
	"io"
	"os"
	"strings"

	"gx/ui"
	"gx/ui/components"

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
		nextYes, decided, accepted, handled := components.UpdateConfirm(msg, m.choiceYes)
		if !handled {
			return m, nil
		}
		m.choiceYes = nextYes
		if decided {
			m.done = true
			m.choiceYes = accepted
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	hint := ui.StyleDim.Render(components.ConfirmHint)
	return tea.NewView(strings.Join([]string{
		m.prompt,
		"",
		components.RenderConfirmChoices(m.choiceYes, m.nerd),
		"  " + hint,
		"",
	}, "\n"))
}
