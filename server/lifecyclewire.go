package server

import (
	"net/http"
	"reflect"
	"time"

	"github.com/elentok/gx/server/lifecycle"
	"github.com/elentok/gx/server/wire"
)

// LogPath is the server's slog JSON file inside stateDir.
func LogPath(stateDir string) string { return lifecycle.LogPath(stateDir) }

// LockHeldBy reports the pid of a live server that holds the state-dir lock.
func LockHeldBy(stateDir string) (int, bool) { return lifecycle.LockHeldBy(stateDir) }

// Rescan rescans the store now, as a ping or the watch would, once the
// migrations have ended: a rescan reads nothing until then.
func (s *Server) Rescan() { s.startup.Rescan() }

// newRestarter builds the restart routes' state from the server's Config.
func (s *Server) newRestarter() *lifecycle.Restarter {
	return lifecycle.NewRestarter(lifecycle.RestartConfig{
		Build: s.cfg.Build, StateDir: s.cfg.StateDir, Env: s.cfg.Env,
		RestartBinary: s.cfg.RestartBinary, SpawnRestart: s.cfg.SpawnRestart,
	}, s.runningAgents)
}

// runningAgents counts loop agents and live interactive sessions.
func (s *Server) runningAgents() int {
	n := s.registry.Count()
	sessions, _ := s.sessions.ListSessions()
	for _, si := range sessions {
		if si.Status.State != wire.SessionExited {
			n++
		}
	}
	return n
}

func restartCheckRoute(s *Server, w http.ResponseWriter, r *http.Request) {
	s.restarter.ServeCheck(w, r)
}

func restartWriteRoute(s *Server, w http.ResponseWriter, r *http.Request) {
	s.restarter.ServeWrite(w, r)
}

// startupHost is lifecycle.StartupHost.
type startupHost struct{ s *Server }

func (h startupHost) MigrationsDone() <-chan struct{} { return h.s.migrator.Done() }
func (h startupHost) EndStartup()                     { h.s.migrator.EndStartup() }
func (h startupHost) Rescan()                         { h.s.idx.Rescan() }
func (h startupHost) RecoverMerges()                  { h.s.merger.RecoverMerges() }
func (h startupHost) ReclaimRuns()                    { h.s.reclaimer.ReclaimRuns() }
func (h startupHost) LogFlushingRunners()             { h.s.reclaimer.LogFlushingRunners() }
func (h startupHost) PruneLogs(now time.Time)         { h.s.sessions.Logs.Prune(now) }

// reloadHost is lifecycle.ReloadHost.
type reloadHost struct{ s *Server }

func (h reloadHost) ApplyLimits(old, next Settings) { h.s.budget.ApplyLimits(old, next) }

func (h reloadHost) ApplyChat(old, next Settings) func() {
	if next.Chat == old.Chat {
		return nil
	}
	return h.s.chat.SetGlobal(next.Chat)
}

func (h reloadHost) ApplyStoreCommits(old, next Settings) func() {
	return h.s.settings.ApplyStoreCommits(old, next)
}

func (h reloadHost) ApplyWatchConfig(old, next Settings) func() {
	return h.s.settings.ApplyWatchConfig(old, next)
}

func (h reloadHost) ReissueCerts(old, next Settings) {
	if certs := h.s.web.Certs(); certs != nil && !reflect.DeepEqual(old.WebAllowedHosts, next.WebAllowedHosts) {
		if err := certs.Reissue(); err != nil {
			h.s.log.Error("web certificate re-issue failed", "err", err)
		}
	}
}

func (h reloadHost) PruneLogsIfRetentionChanged(old, next Settings) {
	if next.LogRetention != old.LogRetention {
		h.s.sessions.Logs.Prune(time.Now())
	}
}

func (h reloadHost) KickGuards() { h.s.guards.Kick() }
