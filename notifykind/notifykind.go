// Package notifykind is the catalog of notification kinds: the 16 rows a
// person checks per channel. It imports only the standard library so config,
// the registry, the router and the server can all read it.
//
// Ids are config values (they sit in config.json kind lists), so they never
// change once shipped. They are not the gate's event types (`notifyKind*`).
package notifykind

// Kind is one catalog row.
type Kind struct {
	// ID is the stable config value.
	ID string
	// Name is the display text.
	Name string
	// Hint is one line that says what the row covers.
	Hint string
	// Default is true when a new install checks the row.
	Default bool
}

// Ids of the catalog rows.
const (
	SessionAlert              = "session-alert"
	DesignQuestion            = "design-question"
	DesignReady               = "design-ready"
	ChatQuestion              = "chat-question"
	PausedTicket              = "paused-ticket"
	OneOffResult              = "one-off-result"
	ConflictResolutionStarted = "conflict-resolution-started"
	EpicComplete              = "epic-complete"
	EpicPaused                = "epic-paused"
	ProgressUpdate            = "progress-update"
	LongTicket                = "long-ticket"
	BudgetAndUsage            = "budget-and-usage"
	Disk                      = "disk"
	ProjectAndTrunk           = "project-and-trunk"
	ServerStarted             = "server-started"
	CLIMessage                = "cli-message"
)

var catalog = []Kind{
	{SessionAlert, "Session alert", "Actionable, blocked, idle and failed sessions", true},
	{DesignQuestion, "Design question", "A design session waits on a round", true},
	{DesignReady, "Design ready", "A design is ready to review", true},
	{ChatQuestion, "Chat question", "An agent asks you a question in chat", true},
	{PausedTicket, "Paused ticket", "A ticket needs a human, including escalations", true},
	{OneOffResult, "One-off result", "The result of a succeeded one-off ticket", true},
	{ConflictResolutionStarted, "Conflict resolution started", "A merge conflict got its own resolution ticket", true},
	{EpicComplete, "Epic complete", "An epic finished or was merged", true},
	{EpicPaused, "Epic paused", "An epic is blocked from merging", true},
	{ProgressUpdate, "Progress update", "Progress while an epic runs", true},
	{LongTicket, "Long ticket", "A ticket ran for many turns", false},
	{BudgetAndUsage, "Budget and usage", "Soft and hard limit, extra usage and the daily summary", true},
	{Disk, "Disk", "Low disk space, resume and clean", true},
	{ProjectAndTrunk, "Project and trunk problems", "Trunk missing or found, project unavailable", true},
	{ServerStarted, "Server started", "The gx server started", true},
	{CLIMessage, "CLI message", "A message sent with gx notify", true},
}

// Channel is one delivery channel. Its id is the config key segment under
// notification-channels.
type Channel struct {
	ID    string
	Label string
}

var channels = []Channel{
	{"desktop", "Desktop"},
	{"web-toast", "Web toast"},
	{"telegram", "Telegram"},
	{"slack", "Slack"},
	{"push", "Push"},
}

// Channels returns the five channels in display order.
func Channels() []Channel { return append([]Channel(nil), channels...) }

// All returns the catalog in row order. The caller may modify the slice.
func All() []Kind { return append([]Kind(nil), catalog...) }

// IDs returns every id in row order.
func IDs() []string {
	ids := make([]string, len(catalog))
	for i, k := range catalog {
		ids[i] = k.ID
	}
	return ids
}

// DefaultIDs returns the ids a new install checks, in row order.
func DefaultIDs() []string {
	var ids []string
	for _, k := range catalog {
		if k.Default {
			ids = append(ids, k.ID)
		}
	}
	return ids
}

// Valid reports whether id is a catalog id.
func Valid(id string) bool {
	for _, k := range catalog {
		if k.ID == id {
			return true
		}
	}
	return false
}
