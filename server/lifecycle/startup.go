package lifecycle

import (
	"context"
	"log/slog"
	"time"
)

// StartupHost is what the root gives the start-up steps.
type StartupHost interface {
	// MigrationsDone is closed once the start-up migrations have ended, pass or fail.
	MigrationsDone() <-chan struct{}
	// EndStartup marks the start-up steps as ended.
	EndStartup()
	Rescan()
	RecoverMerges()
	ReclaimRuns()
	LogFlushingRunners()
	PruneLogs(now time.Time)
}

// Startup runs the steps that read tickets once the migrations have ended.
type Startup struct {
	host StartupHost
	log  *slog.Logger
}

// NewStartup returns the start-up steps over host.
func NewStartup(host StartupHost, log *slog.Logger) *Startup {
	return &Startup{host: host, log: log}
}

// MigrationsEnded reports whether the start-up migrations have ended, pass or
// fail. Until then no code may read or change tickets.
func (st *Startup) MigrationsEnded() bool {
	select {
	case <-st.host.MigrationsDone():
		return true
	default:
		return false
	}
}

// Rescan rescans the store now, as a ping or the watch would, once the
// migrations have ended: a rescan reads nothing until then.
func (st *Startup) Rescan() {
	<-st.host.MigrationsDone()
	st.host.Rescan()
}

// AfterMigrations waits for the migrations to end, then runs the steps that
// read tickets. It reports false when ctx ended first. A project whose migration
// failed is scanned and recovered like the rest.
func (st *Startup) AfterMigrations(ctx context.Context) bool {
	defer st.host.EndStartup()
	select {
	case <-st.host.MigrationsDone():
	case <-ctx.Done():
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	// The first scan comes first: it fills the index the steps below read, and
	// it also raises the parent defects.
	st.step("first-rescan", st.host.Rescan)
	st.step("recover-merges", st.host.RecoverMerges)
	// Running agents stay invisible to a restart, only a little later.
	st.step("reclaim-runs", st.host.ReclaimRuns)
	st.host.LogFlushingRunners()
	st.host.PruneLogs(time.Now())
	return true
}

func (st *Startup) step(name string, run func()) {
	st.log.Info("startup step", "step", name)
	run()
}
