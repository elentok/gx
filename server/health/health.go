// Package health is the system health report: CPU, memory and disk meters, the
// process table behind the cause lists, the CPU spike watch and the start
// incident note.
package health

import (
	"log/slog"
	"time"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/disk"
	"github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/syshealth"
)

// Config is the part of the server config the health subsystem reads. A zero
// duration means the default.
type Config struct {
	Build        string
	StateDir     string
	SessionsDir  string
	SystemHealth syshealth.Reader
	// HealthPollInterval is how often the meters read.
	HealthPollInterval time.Duration
	ProcessSource      syshealth.ProcessReader
	// ProcessSecondReadDelay is the wait between the first and the second
	// process read.
	ProcessSecondReadDelay time.Duration
	// ProcessShareWindow is how long a process read answers later calls.
	ProcessShareWindow time.Duration
	// ProcessCutoff is the age after which a read is too old to be the first
	// of a pair.
	ProcessCutoff time.Duration
	// ProcessWaitCap is the longest a `wait` call blocks.
	ProcessWaitCap time.Duration
}

// SessionMeta is what the owner probe needs from a session's meta file.
type SessionMeta struct {
	Run    int
	Name   string
	Ticket string
}

// Sessions reads a session's meta from its directory. The root implements it
// until sessions move out.
type Sessions interface {
	Meta(dir string) (SessionMeta, error)
}

// Deps is what the health subsystem needs from the root.
type Deps struct {
	Config   Config
	Settings *settings.Holder
	Projects *projects.Registry
	Events   *broker.Broker
	Disks    *disk.Disk
	OneOffs  *queue.OneOffs
	Registry *runs.Registry
	Harness  *harness.Harness
	Sessions Sessions
	Log      *slog.Logger
}

// Health owns the meters, the process cache, the CPU ticket cooldown and the
// session owner cache.
type Health struct {
	cfg      Config
	settings *settings.Holder
	projects *projects.Registry
	events   *broker.Broker
	disks    *disk.Disk
	oneOffs  *queue.OneOffs
	registry *runs.Registry
	harness  *harness.Harness
	sessions Sessions
	log      *slog.Logger

	healthW    healthWatch
	procs      procWatch
	cpuSpikes  cpuSpikes
	ownerCache ownerCache
}

// New builds the health subsystem with every meter waiting for its first reading.
func New(d Deps) *Health {
	h := &Health{cfg: d.Config, settings: d.Settings, projects: d.Projects, events: d.Events, disks: d.Disks,
		oneOffs: d.OneOffs, registry: d.Registry, harness: d.Harness, sessions: d.Sessions, log: d.Log}
	h.healthW.health = wire.Health{CPU: waitingMeter, Memory: waitingMeter, Disk: waitingMeter}
	h.healthW.cpu = meterTrack{high: syshealth.CPUHigh, crit: syshealth.CPUCritical}
	h.healthW.mem = meterTrack{high: syshealth.MemHigh, crit: syshealth.MemCritical}
	return h
}
