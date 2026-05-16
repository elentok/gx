package notify

import tea "charm.land/bubbletea/v2"

// NotifyKind categorizes a notification for styling and dismissal behavior.
type NotifyKind int

const (
	KindInfo     NotifyKind = iota
	KindSuccess             // green, ✔ /
	KindWarning             // orange, ⚠ /
	KindError               // red, ✘ / 󰅙
	KindProgress            // cyan, spinner; requires explicit Close
)

// NotifyMsg is emitted by sub-models as a tea.Cmd result.
// ID is optional for non-progress kinds; required for KindProgress.
// Sending a NotifyMsg with the same ID replaces the existing notification.
type NotifyMsg struct {
	ID      string
	Kind    NotifyKind
	Message string
}

// CloseMsg explicitly dismisses a notification by ID (used to close progress notifications).
type CloseMsg struct {
	ID string
}

func Info(msg string) tea.Cmd {
	return func() tea.Msg { return NotifyMsg{Kind: KindInfo, Message: msg} }
}

func Success(msg string) tea.Cmd {
	return func() tea.Msg { return NotifyMsg{Kind: KindSuccess, Message: msg} }
}

func Warning(msg string) tea.Cmd {
	return func() tea.Msg { return NotifyMsg{Kind: KindWarning, Message: msg} }
}

func Error(msg string) tea.Cmd {
	return func() tea.Msg { return NotifyMsg{Kind: KindError, Message: msg} }
}

func Progress(id, msg string) tea.Cmd {
	return func() tea.Msg { return NotifyMsg{ID: id, Kind: KindProgress, Message: msg} }
}

// Close explicitly dismisses a progress notification by ID.
func Close(id string) tea.Cmd {
	return func() tea.Msg { return CloseMsg{ID: id} }
}
