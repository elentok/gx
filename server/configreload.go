package server

import (
	"reflect"
	"time"

	"github.com/elentok/gx/server/settings"
)

// reloadAppliers is the root's registration. The order is the apply order:
// budget, chat, store commits, swap, then certs, config watch, log prune and
// the runner kick.
func (s *Server) reloadAppliers() settings.Appliers {
	return settings.Appliers{
		Before: []settings.Applier{
			func(old, next Settings) func() { s.budget.ApplyLimits(old, next); return nil },
			func(old, next Settings) func() {
				if next.Chat == old.Chat {
					return nil
				}
				return s.chat.SetGlobal(next.Chat)
			},
			s.settings.ApplyStoreCommits,
		},
		After: []settings.Applier{
			func(old, next Settings) func() {
				if s.webCerts != nil && !reflect.DeepEqual(old.WebAllowedHosts, next.WebAllowedHosts) {
					if err := s.webCerts.Reissue(); err != nil {
						s.log.Error("web certificate re-issue failed", "err", err)
					}
				}
				return nil
			},
			s.settings.ApplyWatchConfig,
			func(old, next Settings) func() {
				if next.LogRetention != old.LogRetention {
					s.sessions.pruneAgentLogs(time.Now())
				}
				return nil
			},
			func(Settings, Settings) func() { s.guards.Kick(); return nil },
		},
	}
}

// banner shows one config-style OS banner and streams it as an event.
func (s *Server) banner(level, title, body string) { s.settings.Banner(level, title, body) }
