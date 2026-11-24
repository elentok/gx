package wire

// Event is one streamed event. Seq is server-wide and contiguous.
type Event struct {
	Seq     uint64 `json:"seq"`
	Type    string `json:"type"`
	Address string `json:"address,omitempty"`
	// Level (EventLevel*) and Message are set only by events that report an
	// outcome, such as config-reloaded.
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`
	// Kind (an events.Kind) tells root-paused causes apart.
	Kind string `json:"kind,omitempty"`
	// Client names the TUI an event is for, by its event-stream client id.
	Client string `json:"client,omitempty"`
	// Set only by EventNotificationToast; Path is a web route, not an Address.
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
	Path  string `json:"path,omitempty"`
}

// EventTicketChanged is emitted once per ticket whose indexed state changed
// (added, edited or removed).
const EventTicketChanged = "ticket-changed"

const (
	EventLevelSuccess = "success"
	EventLevelWarning = "warning"
	EventLevelError   = "error"
)
