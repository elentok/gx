package filter

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Mode is the filter's interaction state.
//
// Filter is deliberately simpler than ui/search: it carries only the query, an
// input box, and whether it is active/focused. It has NO match positions and NO
// match cursor — the host owns the predicate and narrows its own content. See
// ADR 0011 (filter vs search).
type Mode int

const (
	// ModeNone — filter inactive, no query.
	ModeNone Mode = iota
	// ModeInput — filter active and the input box has focus (keystrokes edit the
	// query).
	ModeInput
	// ModeActive — filter active with a kept query but the input is defocused
	// (after enter); the host stays narrowed but keystrokes no longer edit.
	ModeActive
)

type Model struct {
	textinput textinput.Model
	mode      Mode
	query     string
	width     int
}

func NewModel() Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.CursorEnd()

	return Model{
		textinput: ti,
		width:     50,
	}
}

func (m Model) Init() tea.Cmd { return nil }

// Query returns the current filter query.
func (m *Model) Query() string { return m.query }

// HasQuery reports whether there is a non-blank query.
func (m *Model) HasQuery() bool { return strings.TrimSpace(m.query) != "" }

// IsActive reports whether the filter is engaged (input or kept-query mode).
func (m *Model) IsActive() bool { return m.mode != ModeNone }

// InputFocused reports whether keystrokes currently edit the query. The host
// must route keys to the filter first while this is true so its own keybindings
// (e.g. 'q' to close) don't fire mid-typing.
func (m *Model) InputFocused() bool { return m.mode == ModeInput }

// Mode exposes the raw interaction state (mainly for tests).
func (m *Model) Mode() Mode { return m.mode }

// Start activates the filter and focuses the input, seeding it with the current
// query so reopening continues where it left off.
func (m *Model) Start() {
	m.textinput.SetValue(m.query)
	m.textinput.CursorEnd()
	m.textinput.Focus()
	m.mode = ModeInput
}

// Clear deactivates the filter and drops the query.
func (m *Model) Clear() {
	m.query = ""
	m.mode = ModeNone
	m.textinput.SetValue("")
	m.textinput.Blur()
}

// keepAndDefocus retains the query but stops editing (enter).
func (m *Model) keepAndDefocus() {
	m.mode = ModeActive
	m.textinput.Blur()
}

func (m *Model) SetWidth(width int) {
	m.width = width
	// Reserve room for the "/ " prompt.
	m.textinput.SetWidth(max(width-2, 1))
}

// View renders the one-line input bar. It is the host's responsibility to decide
// when to show it (typically only while active).
func (m *Model) View() string {
	return m.textinput.View()
}
