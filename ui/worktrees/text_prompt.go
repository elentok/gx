package worktrees

import (
	"strings"

	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

const textPromptDesiredWidth = 50

// textPrompt is the one-line name prompt behind rename, clone, new and
// new-and-open. The caller picks the title and what a submitted name does.
type textPrompt struct {
	title string
	input textinput.Model
	// submit runs against the page with the trimmed, non-empty name.
	submit func(m Model, name string) (Model, tea.Cmd)
}

func (textPrompt) bottomAnchored() {}

func (p textPrompt) InputFocused() bool { return true }

// Update closes on esc, submits on enter (an empty name just closes) and feeds
// every other key and paste to the input.
func (p textPrompt) Update(msg tea.Msg) (modal, tea.Cmd, modalResult) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			return p, nil, modalResult{close: true}
		case "enter":
			name := strings.TrimSpace(p.input.Value())
			if name == "" {
				return p, nil, modalResult{close: true}
			}
			return p, nil, modalResult{close: true, apply: func(m Model) (Model, tea.Cmd) {
				return p.submit(m, name)
			}}
		}
	} else if _, ok := msg.(tea.PasteMsg); !ok {
		return p, nil, modalResult{unhandled: true}
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return p, cmd, modalResult{}
}

// View renders the framed input overlay.
func (p textPrompt) View(width, _ int) string {
	outerW := min(textPromptDesiredWidth, width*80/100)
	ti := p.input
	ti.SetWidth(outerW - 2 - 2) // minus border and padding
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         p.title,
		Body:          ti.View(),
		Width:         outerW,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		TitleInBorder: true,
	})
}
