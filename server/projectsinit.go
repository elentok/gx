package server

import (
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/wire"
)

// projectsEnvOf is how the projects types see the Server's config: they hold
// this getter and call it on every read, so a later change to the config shows.
func (s *Server) projectsEnvOf() projects.Env {
	c := &s.cfg
	return projects.Env{
		TicketStore: c.TicketStore, StateDir: c.StateDir, Env: c.Env, Migrations: c.Migrations,
		MigrationFailurePollInterval: c.MigrationFailurePollInterval, Chat: c.Chat, GuardPoll: c.GuardPoll,
		InstallDeps: c.InstallDeps, RateLimitResetBuffer: c.RateLimitResetBuffer, RunnerWait: c.RunnerWait, Sleep: c.Sleep,
	}
}

// initProjects builds the projects Registry, Migrator and Checks from the
// Server's own fields. Tests that build a bare Server call it too.
func (s *Server) initProjects() {
	s.projects = projects.NewRegistry(s.projectsEnvOf, s.log, s.idx, &s.settings, s.chat, s)
	s.migrator = projects.NewMigrator(s.projectsEnvOf, s.log, s)
	s.checks = projects.NewChecks(s.projectsEnvOf, s.projects, s.guards)
}

// withProjects is initProjects for a bare Server literal in a test.
func withProjects(s *Server) *Server {
	s.initProjects()
	return s
}

// BusyAddresses implements projects.RegistryBack.
func (s *Server) BusyAddresses() []string {
	var out []string
	for _, it := range s.queued.list() {
		out = append(out, it.Address)
	}
	for _, r := range s.registry.List() {
		out = append(out, r.Address)
	}
	return out
}

// FailuresChanged implements projects.MigratorBack.
func (s *Server) FailuresChanged() {
	s.events.Publish(wire.EventMigrationFailuresChanged, "")
	s.guards.Kick() // a clear ends a Migration block: claim now, not at the next tick
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

// StartupDone closes once the startup steps, the first rescan among them, ended.
func (s *Server) StartupDone() <-chan struct{} { return s.migrator.StartupDone() }

// WorktreeDir is where the project's iteration worktrees live.
func (s *Server) WorktreeDir(project string) string { return s.projects.WorktreeDir(project) }
