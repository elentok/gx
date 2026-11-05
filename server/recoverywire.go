package server

import (
	srvrecovery "github.com/elentok/gx/server/recovery"
	"github.com/elentok/gx/server/ticketwrite"
)

// newRecovery needs s.writer built; the pause hold and settings are the
// server's own, which it also reads.
func newRecovery(s *Server) *srvrecovery.Recovery {
	return srvrecovery.New(srvrecovery.Deps{
		Log: s.log, Settings: &s.settings, Projects: s.projects, Migrator: s.migrator,
		Index: s.idx, Queue: s.queue, Events: s.events, Guards: s.guards, Registry: s.registry,
		Harness: s.harness, Chat: s.chat, Writer: s.writer, Hold: &s.pauseHold,
		CompactRewaitTimeout: s.cfg.CompactRewaitTimeout, Host: recoveryHost{s},
	})
}

// recoveryHost is recovery.Host.
type recoveryHost struct{ s *Server }

func (h recoveryHost) LiveIteration(address string) (ticketwrite.LiveIter, bool) {
	return liveIteration(h.s, address)
}

// EnsureDraftEpic is queue.Host's.
func (s *Server) EnsureDraftEpic(epicPath string) error { return srvrecovery.EnsureDraftEpic(epicPath) }
