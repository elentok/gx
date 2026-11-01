package server

import (
	"context"
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
	for _, dir := range dirs {
		if ctx.Err() != nil {
			return
		}
		s.migrateProject(reg, dir)
	}
}

func (s *Server) migrateProject(reg *migrations.Registry, dir string) {
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
