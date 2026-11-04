package harness

import (
	"log/slog"
	"sync"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Host is what the harness choice needs from the rest of the server: the back
// edges to sessions and to the ticket reader. The root implements it; tests use
// a fake.
type Host interface {
	ListSessions() ([]wire.SessionInfo, error)
	FreshTicket(addr tickets.Address) (tickets.Epic, tickets.Ticket, bool)
}

// Config is the part of the server Config the harness choice reads.
type Config struct {
	Env         []string
	LookPath    func(program string) (string, error)
	Runner      agentrunner.Runner
	RunnerFor   func(name, project string) agentrunner.Runner
	AgentRunner string
	ConfigPath  string
}

// Harness owns the harness and runner choice. Its HTTP handlers are Routes.
type Harness struct {
	cfg      Config
	host     Host
	log      *slog.Logger
	idx      *index.Index
	projects *projects.Registry
	queue    *queue.Queue
	registry *runs.Registry
	settings *settings.Holder

	// logGiven serializes GiveLog.
	logGiven sync.Mutex
	// recorded caches sessionRecorded, which globs every Claude project dir.
	recorded recordedCache
}

// Deps is what New needs.
type Deps struct {
	Config   Config
	Host     Host
	Log      *slog.Logger
	Index    *index.Index
	Projects *projects.Registry
	Queue    *queue.Queue
	Registry *runs.Registry
	Settings *settings.Holder
}

// New is the harness choice: which harness and runner a ticket or session
// launches under, and the Default harness setting.
func New(d Deps) *Harness {
	return &Harness{cfg: d.Config, host: d.Host, log: d.Log, idx: d.Index, projects: d.Projects, queue: d.Queue, registry: d.Registry, settings: d.Settings}
}
