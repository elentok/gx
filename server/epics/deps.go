package epics

import (
	"log/slog"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Harness answers which agent session a ticket has and what to call its harness.
type Harness interface {
	LiveSession(addr tickets.Address) (agentrunner.Runner, agentrunner.Session, bool, error)
	LabelFor(addr tickets.Address, agent string, status string) wire.HarnessLabel
}

// Projects lists a project's epics and names the harness it launches.
type Projects interface {
	Epics(project string) ([]tickets.Epic, error)
	LaunchHarness(project string) string
}

// Chat finds a ticket's pty directory.
type Chat interface {
	TicketPtyDir(addr tickets.Address) (string, bool)
}

// Settings is the live settings snapshot.
type Settings interface {
	Load() *settings.Settings
}

// Writer finds a ticket by address.
type Writer interface {
	FindTicket(address string) (queue.TicketRef, bool, error)
}

// Runs knows when a run started and which launch step it is in.
type Runs interface {
	StartedAtOf(address string) (time.Time, bool)
	LaunchStepOf(address string) (label string, position int)
}

// History reads a ticket's event history.
type History interface {
	ReadHistory(addr tickets.Address) ([]runlog.Event, error)
}

// Queue is the ralph-loop queue as the read models see it.
type Queue interface {
	List() []wire.QueueItem
	RootQueued(root queue.RootRef) bool
}

// Pauses is the recovery-hold state per ticket.
type Pauses interface {
	State(key string) string
}

// Sessions lists the live sessions.
type Sessions interface {
	ListSessions() ([]wire.SessionInfo, error)
}

// Deps is what the epic read models need from the root server. The function
// fields are root helpers that stay up.
type Deps struct {
	Log         *slog.Logger
	TicketStore string

	Harness  Harness
	Projects Projects
	Chat     Chat
	Settings Settings
	Writer   Writer
	Runs     Runs
	History  History
	Queue    Queue
	Pauses   Pauses
	Sessions Sessions

	// CapHeld gives the cap verdict of the epic's next ready ticket, when a cap
	// holds it back.
	CapHeld           func(root queue.RootRef, e tickets.Epic) (wire.Explanation, bool)
	AutoFFMerge       func(project string) bool
	CheckWaitingLabel func(address string) string
	AgentGone         func(address string) (bool, error)
	AmbiguousBlockers func(addr tickets.Address) []string
	DesignUncharted   func(epicAddr string, e tickets.Epic) (bool, error)
}

// Epics serves the epic and ticket read models.
type Epics struct {
	Deps
}

// New builds the read models over d.
func New(d Deps) *Epics { return &Epics{Deps: d} }
