package modal

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Credential asks the user for a credential a running job is waiting on.
type Credential struct {
	input  textinput.Model
	text   string
	runner *components.CommandRunner
}

// NewCredential asks for the credential prompt, answering it through runner.
func NewCredential(prompt components.CredentialPrompt, runner *components.CommandRunner) Credential {
	ti := textinput.New()
	ti.Focus()
	if prompt.Kind == components.PromptKindSecret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '*'
	}
	return Credential{input: ti, text: prompt.Text, runner: runner}
}

func (c Credential) Update(msg tea.Msg) (Modal, tea.Cmd, Result) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			if c.runner != nil {
				c.runner.Cancel()
			}
			return c, nil, Result{Close: true}
		case "enter":
			if c.runner != nil {
				_ = c.runner.SubmitPromptInput(c.input.Value())
			}
			return c, nil, Result{Close: true}
		}
	case tea.PasteMsg:
	default:
		return c, nil, Result{Unhandled: true}
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd, Result{}
}

func (c Credential) InputFocused() bool { return true }

func (c Credential) View(_, _ int) string {
	input := c.input.View()
	if input == "" {
		input = " "
	}
	return components.RenderInputModal(
		"Credential Required",
		c.text,
		input,
		ui.HintSubmitCancel(),
		ui.ColorBorder,
		ui.ColorGreen,
		ui.ColorGray,
		0,
	)
}
