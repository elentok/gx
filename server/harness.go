package server

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

// HarnessHost is what the harness choice needs from the rest of the server: the
// back edges to sessions and to the ticket reader. The root implements it;
// tests use a fake.
type HarnessHost interface {
	ListSessions() ([]wire.SessionInfo, error)
	FreshTicket(addr tickets.Address) (tickets.Epic, tickets.Ticket, bool)
}

// HarnessConfig is the part of Config the harness choice reads.
type HarnessConfig struct {
	Env         []string
	LookPath    func(program string) (string, error)
	Runner      agentrunner.Runner
	RunnerFor   func(name, project string) agentrunner.Runner
	AgentRunner string
	ConfigPath  string
}

// queueHost is queue.Host: the harness methods the queue reads, and the rest
// from the root.
type queueHost struct {
	*Server
	*Harness
}

// Harness is the server's harness choice. Tests outside the package read it
// through here.
func (s *Server) Harness() *Harness { return s.harness }

// Harness owns the harness and runner choice. The HTTP handlers stay on Server.
type Harness struct {
	cfg      HarnessConfig
	host     HarnessHost
	log      *slog.Logger
	idx      *index.Index
	projects *projects.Registry
	queue    *queue.Queue
	registry *runs.Registry
	settings *settings.Holder

	// logGiven serializes giveLog.
	logGiven sync.Mutex
	// recorded caches sessionRecorded, which globs every Claude project dir.
	recorded recordedCache
}

// HarnessDeps is what NewHarness needs.
type HarnessDeps struct {
	Config   HarnessConfig
	Host     HarnessHost
	Log      *slog.Logger
	Index    *index.Index
	Projects *projects.Registry
	Queue    *queue.Queue
	Registry *runs.Registry
	Settings *settings.Holder
}

// NewHarness is the harness choice: which harness and runner a ticket or
// session launches under, and the Default harness setting.
func NewHarness(d HarnessDeps) *Harness {
	return &Harness{cfg: d.Config, host: d.Host, log: d.Log, idx: d.Index, projects: d.Projects, queue: d.Queue, registry: d.Registry, settings: d.Settings}
}
