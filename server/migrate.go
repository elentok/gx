package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/projectlock"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// migratorBack is what the Migrator reads from the Server.
type migratorBack interface {
	// failuresChanged announces a change to the saved migration failures.
	failuresChanged()
}

// Migrator runs the start-up migration of every project and the retries, and
// reads the failures they leave.
type Migrator struct {
	cfg  *Config // the Server's own, so a later change to it shows here
	log  *slog.Logger
	back migratorBack
	// done closes when the start-up migration run ends.
	done chan struct{}
	// startupDone closes when the steps that run after the migrations (the
	// first scan, merge recovery, run reclaim) have ended; claiming waits for it.
	startupDone chan struct{}
	// now counts the migration runs in flight per project name (the start-up
	// run and Retry); Retry is refused while a project's count is above zero.
	mu  sync.Mutex
	now map[string]int
}

// MigrationsDone is closed when the start-up migration of every project has
// ended, whether each project passed or failed. It is closed even when there is
// no ticket store.
func (s *Server) MigrationsDone() <-chan struct{} { return s.migrator.Done() }

// MigrationRunning reports whether a migration run of project is in flight.
func (s *Server) MigrationRunning(project string) bool { return s.migrator.Running(project) }

// MigrationFailures reads the saved failed migrations of the projects now in
// the ticket store. The CLI writes the record, so each call reads the file.
func (s *Server) MigrationFailures() []wire.MigrationFailure { return s.migrator.Failures() }

// failuresChanged implements migratorBack.
func (s *Server) failuresChanged() {
	s.events.Publish(wire.EventMigrationFailuresChanged, "")
	s.guards.Kick() // a clear ends a Migration block: claim now, not at the next tick
}

// Done is closed when the start-up migration run has ended.
func (m *Migrator) Done() <-chan struct{} { return m.done }

// migrating reports whether the start-up migration run is still going.
func (m *Migrator) Migrating() bool {
	select {
	case <-m.done:
		return false
	default:
		return true
	}
}

func (m *Migrator) Steps() *migrations.Registry {
	if m.cfg.Migrations != nil {
		return m.cfg.Migrations
	}
	return migrations.Default
}

// migrateAll migrates the projects one after another. A failure is saved and
// the next project still runs. It does not stop a step in progress when ctx
// ends; it only starts no further project.
func (m *Migrator) MigrateAll(ctx context.Context) {
	defer close(m.done)
	if m.cfg.TicketStore == "" {
		return
	}
	dirs, err := tickets.ProjectDirs(m.cfg.TicketStore)
	if err != nil {
		m.log.Error("list projects to migrate", "err", err)
		return
	}
	reg := m.Steps()
	if unknown, err := reg.UnknownApplied(dirs...); err != nil {
		m.log.Warn("read applied migrations", "err", err)
	} else if len(unknown) > 0 {
		m.log.Warn("applied migrations unknown to this gx (a newer version ran them)", "names", strings.Join(unknown, ", "))
	}
	pass := &migrations.Pass{}
	for _, dir := range dirs {
		if ctx.Err() != nil {
			return
		}
		name := tickets.ProjectName(dir)
		m.mu.Lock()
		m.markLocked(name, 1)
		m.mu.Unlock()
		m.migrateProject(reg, dir, pass)
		m.End(name)
	}
}

func (s *Server) retryMigrationWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.RetryMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, s.migrator.Retry(req.Project))
}

// retryMigration runs the project's pending steps in the background, the same
// code path as the start-up run.
func (m *Migrator) Retry(project string) wire.RetryMigrationResult {
	dir := ""
	if m.cfg.TicketStore != "" {
		dirs, _ := tickets.ProjectDirs(m.cfg.TicketStore)
		for _, d := range dirs {
			if tickets.ProjectName(d) == project {
				dir = d
			}
		}
	}
	if dir == "" {
		return wire.RetryMigrationResult{Refused: true, Reason: wire.ReasonRetryUnknownProject, Message: "no project " + project}
	}
	if !m.Begin(project) {
		return wire.RetryMigrationResult{Refused: true, Reason: wire.ReasonRetryRunning, Message: "project " + project + " is migrating"}
	}
	go func() {
		defer m.End(project)
		m.migrateProject(m.Steps(), dir, &migrations.Pass{})
	}()
	return wire.RetryMigrationResult{Started: true}
}

// beginMigration marks project as migrating; it reports false when another run
// of that project is already in flight.
func (m *Migrator) Begin(project string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.now[project] > 0 {
		return false
	}
	m.markLocked(project, 1)
	return true
}

// Running reports whether a migration run of project is in flight.
func (m *Migrator) Running(project string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now[project] > 0
}

func (m *Migrator) End(project string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markLocked(project, -1)
}

func (m *Migrator) markLocked(project string, delta int) {
	if m.now == nil {
		m.now = map[string]int{}
	}
	m.now[project] += delta
}

func (m *Migrator) migrateProject(reg *migrations.Registry, dir string, pass *migrations.Pass) {
	name := tickets.ProjectName(dir)
	unlock, err := projectlock.Exclusive(dir)
	if err != nil {
		m.recordFailure(name, err)
		return
	}
	defer unlock()
	changes, _, err := reg.Run(migrations.Env{
		Dirs: config.DirsFrom(m.cfg.Env), Root: dir, LookupRoot: dir,
		AppliedPath: filepath.Join(dir, migrations.AppliedFile),
		SkipApplied: true,
		StateDir:    m.cfg.StateDir, Pass: pass,
	})
	for _, c := range changes {
		for _, note := range c.Notes {
			m.log.Info("migration note", "project", name, "path", c.Path, "note", note)
		}
	}
	if err != nil {
		m.recordFailure(name, err)
		return
	}
	// Clear takes a lock file in the state dir; skip it when there is no record.
	if recorded, err := migratefail.Read(m.cfg.StateDir, []string{name}); err == nil && len(recorded) == 0 {
		return
	}
	if err := migratefail.Clear(m.cfg.StateDir, name); err != nil {
		m.log.Error("clear migration failure", "project", name, "err", err)
	}
}

func (m *Migrator) recordFailure(project string, err error) {
	m.log.Error("migration failed", "project", project, "err", err)
	if recErr := migratefail.Record(m.cfg.StateDir, project, err.Error()); recErr != nil {
		m.log.Error("save migration failure", "project", project, "err", recErr)
	}
}
