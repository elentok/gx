package wire

// ReasonFocusNotConnected refuses a focus report from a client with no open
// event stream: the report would never expire.
const ReasonFocusNotConnected = "not-connected"

// EventSessionFocus asks a TUI to show a session; its Address is the session
// id and its Client the TUI.
const EventSessionFocus = "session-focus"

// FocusReport is a TUI's terminal focus, the session it has on screen (empty
// for none) and its terminal app. Client names the TUI's event stream
// (/v1/events?client=); the report counts only while that stream is open.
type FocusReport struct {
	Client      string `json:"client"`
	Focused     bool   `json:"focused,omitempty"`
	Session     string `json:"session,omitempty"`
	TermProgram string `json:"term_program,omitempty"`
	// Web marks a web tab's report. Only the web mux sets it, never the body.
	Web bool `json:"-"`
}
