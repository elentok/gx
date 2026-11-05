package rowactions

// TicketTarget is the ticket row a Context menu was opened on. EpicName is
// the bare epic the herdr launches want.
type TicketTarget struct{ Address, EpicName, TicketID string }

// ServerNotRunning is the reason the menus give for a write that needs the
// server while it is down.
const ServerNotRunning = "server not running"

// NoAgentLog is the reason Watch and Transcript are dim when no agent log
// exists.
const NoAgentLog = "no agent log"
