package modal

import (
	"strings"

	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

const textPromptDesiredWidth = 50

// TextPrompt is the one-line name prompt behind rename, clone, new and
// new-and-open. The caller picks the title and what a submitted name does.
type TextPrompt struct {
	title string
	input textinput.Model
	// kind picks the page command the trimmed, non-empty name feeds.
	kind NameKind
}

// NewRenamePrompt opens a prompt pre-filled with the current worktree name.
func NewRenamePrompt(currentName string) TextPrompt {
	return TextPrompt{title: "Rename Worktree", input: prefilledInput(currentName), kind: NameRename}
}

// NewClonePrompt opens a prompt pre-filled with the source worktree name.
func NewClonePrompt(sourceName string) TextPrompt {
	return TextPrompt{title: "Clone Worktree", input: prefilledInput(sourceName), kind: NameClone}
}

// NewWorktreePrompt opens the new-worktree prompt; kind (NameNew or
// NameNewAndOpen) picks the command a submitted name runs.
func NewWorktreePrompt(kind NameKind) TextPrompt {
	ti := textinput.New()
	ti.Placeholder = "worktree-name"
	ti.Focus()
	return TextPrompt{title: "New Worktree", input: ti, kind: kind}
}

// prefilledInput is a focused textinput with the cursor at the end, so the
// user can backspace or edit inline.
func prefilledInput(value string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(value)
	ti.CursorEnd()
	ti.Focus()
	return ti
}

// Value is the text currently typed into the prompt.
func (p TextPrompt) Value() string { return p.input.Value() }

func (TextPrompt) BottomAnchored() {}

func (p TextPrompt) InputFocused() bool { return true }

// Update closes on esc, submits on enter (an empty name just closes) and feeds
// every other key and paste to the input.
func (p TextPrompt) Update(msg tea.Msg) (Modal, tea.Cmd, Result) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			return p, nil, Result{Close: true}
		case "enter":
			name := strings.TrimSpace(p.input.Value())
			if name == "" {
				return p, nil, Result{Close: true}
			}
			return p, nil, Result{Close: true, Action: SubmitName{Kind: p.kind, Name: name}}
		}
	} else if _, ok := msg.(tea.PasteMsg); !ok {
		return p, nil, Result{Unhandled: true}
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return p, cmd, Result{}
}

// View renders the framed input overlay.
func (p TextPrompt) View(width, _ int) string {
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
