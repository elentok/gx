package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/projectlock"
	"github.com/elentok/gx/tickets"
)

// MigrationsDone is closed when the start-up migration of every project has
// ended, whether each project passed or failed. It is closed even when there is
// no ticket store.
func (s *Server) MigrationsDone() <-chan struct{} { return s.migrationsDone }

// migrating reports whether the start-up migration run is still going.
func (s *Server) migrating() bool {
	select {
	case <-s.migrationsDone:
		return false
	default:
		return true
	}
}

func (s *Server) migrationRegistry() *migrations.Registry {
	if s.cfg.Migrations != nil {
		return s.cfg.Migrations
	}
	return migrations.Default
}

// migrateAll migrates the projects one after another. A failure is saved and
// the next project still runs. It does not stop a step in progress when ctx
// ends; it only starts no further project.
func (s *Server) migrateAll(ctx context.Context) {
	defer close(s.migrationsDone)
	if s.cfg.TicketStore == "" {
		return
	}
	dirs, err := tickets.ProjectDirs(s.cfg.TicketStore)
	if err != nil {
		s.log.Error("list projects to migrate", "err", err)
		return
	}
	reg := s.migrationRegistry()
	if unknown, err := reg.UnknownApplied(dirs...); err != nil {
		s.log.Warn("read applied migrations", "err", err)
	} else if len(unknown) > 0 {
		s.log.Warn("applied migrations unknown to this gx (a newer version ran them)", "names", strings.Join(unknown, ", "))
	}
	pass := &migrations.Pass{}
	for _, dir := range dirs {
		if ctx.Err() != nil {
			return
		}
		name := tickets.ProjectName(dir)
		s.migratingMu.Lock()
		s.markMigrationLocked(name, 1)
		s.migratingMu.Unlock()
		s.migrateProject(reg, dir, pass)
		s.endMigration(name)
	}
}

// Refusal reasons of the retry-migration route.
const (
	ReasonRetryRunning        = "migrating"
	ReasonRetryUnknownProject = "unknown-project"
)

// RetryMigrationRequest is the body of POST /v1/projects/retry-migration.
type RetryMigrationRequest struct {
	Project string `json:"project"`
}

// RetryMigrationResult answers it: Started, or Refused with a Reason.
type RetryMigrationResult struct {
	Started bool   `json:"started"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

func (s *Server) retryMigrationWrite(w http.ResponseWriter, r *http.Request) {
	var req RetryMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, s.retryMigration(req.Project))
}

// retryMigration runs the project's pending steps in the background, the same
// code path as the start-up run.
func (s *Server) retryMigration(project string) RetryMigrationResult {
	dir := ""
	if s.cfg.TicketStore != "" {
		dirs, _ := tickets.ProjectDirs(s.cfg.TicketStore)
		for _, d := range dirs {
			if tickets.ProjectName(d) == project {
				dir = d
			}
		}
	}
	if dir == "" {
		return RetryMigrationResult{Refused: true, Reason: ReasonRetryUnknownProject, Message: "no project " + project}
	}
	if !s.beginMigration(project) {
		return RetryMigrationResult{Refused: true, Reason: ReasonRetryRunning, Message: "project " + project + " is migrating"}
	}
	go func() {
		defer s.endMigration(project)
		s.migrateProject(s.migrationRegistry(), dir, &migrations.Pass{})
	}()
	return RetryMigrationResult{Started: true}
}

// beginMigration marks project as migrating; it reports false when another run
// of that project is already in flight.
func (s *Server) beginMigration(project string) bool {
	s.migratingMu.Lock()
	defer s.migratingMu.Unlock()
	if s.migratingNow[project] > 0 {
		return false
	}
	s.markMigrationLocked(project, 1)
	return true
}

// MigrationRunning reports whether a migration run of project is in flight.
func (s *Server) MigrationRunning(project string) bool {
	s.migratingMu.Lock()
	defer s.migratingMu.Unlock()
	return s.migratingNow[project] > 0
}

func (s *Server) endMigration(project string) {
	s.migratingMu.Lock()
	defer s.migratingMu.Unlock()
	s.markMigrationLocked(project, -1)
}

func (s *Server) markMigrationLocked(project string, delta int) {
	if s.migratingNow == nil {
		s.migratingNow = map[string]int{}
	}
	s.migratingNow[project] += delta
}

func (s *Server) migrateProject(reg *migrations.Registry, dir string, pass *migrations.Pass) {
	name := tickets.ProjectName(dir)
	unlock, err := projectlock.Exclusive(dir)
	if err != nil {
		s.recordMigrationFailure(name, err)
		return
	}
	defer unlock()
	changes, _, err := reg.Run(migrations.Env{
		Dirs: config.DirsFrom(s.cfg.Env), Root: dir, LookupRoot: dir,
		AppliedPath: filepath.Join(dir, migrations.AppliedFile),
		SkipApplied: true,
		StateDir:    s.cfg.StateDir, Pass: pass,
	})
	for _, c := range changes {
		for _, note := range c.Notes {
			s.log.Info("migration note", "project", name, "path", c.Path, "note", note)
		}
	}
	if err != nil {
		s.recordMigrationFailure(name, err)
		return
	}
	// Clear takes a lock file in the state dir; skip it when there is no record.
	if recorded, err := migratefail.Read(s.cfg.StateDir, []string{name}); err == nil && len(recorded) == 0 {
		return
	}
	if err := migratefail.Clear(s.cfg.StateDir, name); err != nil {
		s.log.Error("clear migration failure", "project", name, "err", err)
	}
}

func (s *Server) recordMigrationFailure(project string, err error) {
	s.log.Error("migration failed", "project", project, "err", err)
	if recErr := migratefail.Record(s.cfg.StateDir, project, err.Error()); recErr != nil {
		s.log.Error("save migration failure", "project", project, "err", recErr)
	}
}
