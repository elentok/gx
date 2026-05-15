package output

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
)

// Model owns a scrollable command-output modal.
// The parent stores nothing — it calls Set to record output and Open to display it.
type Model struct {
	IsOpen  bool
	title   string
	content string
	vp      viewport.Model
}

// New returns a zero-value Model.
func New() Model { return Model{} }

// Set records output for later display without opening the modal.
// Empty content is ignored so a previous result isn't overwritten with silence.
func (m *Model) Set(title, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	m.title = title
	m.content = content
}

// HasContent reports whether there is recorded output to show.
func (m Model) HasContent() bool { return m.content != "" }

// Open sizes a viewport and opens the modal. Call after Set.
func (m *Model) Open(containerWidth, containerHeight int) {
	vpW := max(56, min(110, containerWidth*2/3))
	vpH := max(8, containerHeight/2-4)
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(m.content)
	m.vp = vp
	m.IsOpen = true
}

// Update handles all messages while the modal is open.
// Returns the updated model and any command. The modal closes on esc/enter/q.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch kp.String() {
	case "esc", "enter", "q":
		m.IsOpen = false
		return m, nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// View renders the modal. Returns "" when not open.
func (m Model) View() string {
	if !m.IsOpen {
		return ""
	}
	title := m.title
	if title == "" {
		title = "Command output"
	}
	return components.RenderOutputModal(
		title,
		m.vp.View(),
		ui.HintDismissAndScroll(),
		ui.ColorYellow,
		ui.ColorYellow,
		ui.ColorSubtle,
		m.vp.Width(),
	)
}
