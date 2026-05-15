package creds

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
)

// Result is returned from Update when the prompt is resolved.
type Result struct {
	Decided   bool
	Value     string // non-empty when submitted
	Cancelled bool
}

// Model is a self-contained credential input modal.
type Model struct {
	IsOpen bool
	prompt string
	secret bool
	input  textinput.Model
}

// New returns a zero-value Model.
func New() Model { return Model{} }

// Open shows the credential prompt.
func (m *Model) Open(p components.CredentialPrompt) {
	m.prompt = p.Text
	m.secret = p.Kind == components.PromptKindSecret
	ti := textinput.New()
	ti.Focus()
	if m.secret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '*'
	}
	m.input = ti
	m.IsOpen = true
}

// Update handles key events and textinput updates. Returns Result with
// Decided=true once the user submits or cancels.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd, Result{}
	}
	switch kp.String() {
	case "esc":
		m.IsOpen = false
		return m, nil, Result{Decided: true, Cancelled: true}
	case "enter":
		val := m.input.Value()
		m.IsOpen = false
		return m, nil, Result{Decided: true, Value: val}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd, Result{}
}

// View renders the credential input modal.
func (m Model) View(width int) string {
	input := m.input.View()
	if input == "" {
		input = " "
	}
	return components.RenderInputModal(
		"Credential Required",
		m.prompt,
		input,
		ui.HintSubmitCancel(),
		ui.ColorBlue,
		ui.ColorBlue,
		ui.ColorSubtle,
		width,
	)
}
