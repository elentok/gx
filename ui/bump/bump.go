package bump

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
)

type phase int

const (
	phasePick         phase = iota
	phaseTagging
	phaseConfirmPush
	phaseFailed
)

type tagDoneMsg struct{ err error }

// Result is returned on each Update call when the modal finishes.
type Result struct {
	Done          bool
	PushRequested bool
	Err           error
}

// Model owns the bump lifecycle: pick version → create tag → confirm push.
type Model struct {
	IsOpen bool

	root    string
	lastTag string
	newTag  string

	phase   phase
	menu    components.MenuState
	pushYes bool
	spinner spinner.Model
	failErr error
}

// New returns a zero-value Model.
func New() Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{spinner: sp}
}

// Open reads the current tag, builds version options, and opens the modal.
// Returns an error only if the current tag cannot be parsed.
func (m *Model) Open(root string) error {
	lastTag := git.LastTag(root)
	major, minor, patch, err := git.ParseVersion(lastTag)
	if err != nil {
		return err
	}

	m.root = root
	m.lastTag = lastTag
	m.newTag = ""
	m.phase = phasePick
	m.pushYes = true
	m.failErr = nil
	m.menu = components.MenuState{
		Items: []components.MenuItem{
			{Label: "patch", Detail: fmt.Sprintf("%s → v%d.%d.%d", lastTag, major, minor, patch+1)},
			{Label: "minor", Detail: fmt.Sprintf("%s → v%d.%d.%d", lastTag, major, minor+1, 0)},
			{Label: "major", Detail: fmt.Sprintf("%s → v%d.%d.%d", lastTag, major+1, 0, 0)},
		},
	}
	m.IsOpen = true
	return nil
}

// Update handles all messages while the modal is open.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tagDoneMsg:
		if msg.err != nil {
			m.phase = phaseFailed
			m.failErr = msg.err
			return m, nil, Result{}
		}
		m.phase = phaseConfirmPush
		return m, nil, Result{}

	case spinner.TickMsg:
		if m.phase != phaseTagging {
			return m, nil, Result{}
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd, Result{}
	}
	return m, nil, Result{}
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd, Result) {
	switch m.phase {
	case phasePick:
		next, decided, accepted, handled := components.UpdateMenu(msg, m.menu)
		if !handled {
			return m, nil, Result{}
		}
		m.menu = next
		if !decided {
			return m, nil, Result{}
		}
		if !accepted {
			m.IsOpen = false
			return m, nil, Result{Done: true}
		}
		m.newTag = m.selectedNewTag()
		m.phase = phaseTagging
		return m, tea.Batch(m.cmdCreateTag(), m.spinner.Tick), Result{}

	case phaseConfirmPush:
		nextYes, decided, accepted, handled := components.UpdateConfirm(msg, m.pushYes)
		if !handled {
			return m, nil, Result{}
		}
		m.pushYes = nextYes
		if !decided {
			return m, nil, Result{}
		}
		m.IsOpen = false
		return m, nil, Result{Done: true, PushRequested: accepted}

	case phaseFailed:
		switch msg.String() {
		case "esc", "enter", "q":
			m.IsOpen = false
			return m, nil, Result{Done: true, Err: m.failErr}
		}
	}
	return m, nil, Result{}
}

func (m Model) cmdCreateTag() tea.Cmd {
	root, tag := m.root, m.newTag
	return func() tea.Msg {
		err := git.CreateAnnotatedTag(root, tag, "Release "+tag)
		return tagDoneMsg{err: err}
	}
}

func (m Model) selectedNewTag() string {
	if m.menu.Cursor < 0 || m.menu.Cursor >= len(m.menu.Items) {
		return ""
	}
	item := m.menu.Items[m.menu.Cursor]
	// Detail format: "v1.2.3 → v1.2.4" — extract the part after " → "
	for i := len(item.Detail) - 1; i >= 0; i-- {
		if item.Detail[i] == ' ' && i+1 < len(item.Detail) && item.Detail[i+1] == 'v' {
			return item.Detail[i+1:]
		}
	}
	return ""
}

// View renders the modal for the current phase.
func (m Model) View(width int) string {
	w := modalWidth(width)
	switch m.phase {
	case phasePick:
		return components.RenderMenuModal(
			"Bump Version",
			"Current: "+ui.StyleDim.Render(m.lastTag),
			m.menu,
			"",
			ui.ColorYellow, ui.ColorYellow, ui.ColorSubtle, ui.ColorGreen,
			w,
		)

	case phaseTagging:
		body := m.spinner.View() + " creating tag " + ui.StyleTitle.Render(m.newTag) + "..."
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Bump Version",
			Body:        body,
			Width:       w,
			BorderColor: ui.ColorYellow,
			TitleColor:  ui.ColorYellow,
			HintColor:   ui.ColorSubtle,
		})

	case phaseConfirmPush:
		body := "Created tag " + ui.StyleTitle.Render(m.newTag) + "\n\nPush to origin?"
		body += "\n\n" + components.RenderConfirmChoices(m.pushYes, false)
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Bump Version",
			Body:        body,
			Hint:        components.ConfirmHint,
			Width:       w,
			BorderColor: ui.ColorYellow,
			TitleColor:  ui.ColorYellow,
			HintColor:   ui.ColorSubtle,
		})

	case phaseFailed:
		body := ui.StyleWarning.Render(m.failErr.Error()) + "\n\n" + ui.StyleMuted.Render("press esc to dismiss")
		return ui.RenderModalFrame(ui.ModalFrameOptions{
			Title:       "Bump Version",
			Body:        body,
			Width:       w,
			BorderColor: ui.ColorRed,
			TitleColor:  ui.ColorRed,
			HintColor:   ui.ColorSubtle,
		})
	}
	return ""
}

func modalWidth(width int) int {
	w := width / 2
	if w < 56 {
		w = 56
	}
	return w
}
