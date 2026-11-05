package lifecycle

import (
	"github.com/elentok/gx/server/settings"
)

// ReloadHost is what the root gives the config-reload apply steps.
type ReloadHost interface {
	ApplyLimits(old, next settings.Settings)
	// ApplyChat applies a changed chat setting; it returns nil when nothing changed.
	ApplyChat(old, next settings.Settings) func()
	ApplyStoreCommits(old, next settings.Settings) func()
	// ReissueCerts re-issues the web certificate when the allowed hosts changed.
	ReissueCerts(old, next settings.Settings)
	ApplyWatchConfig(old, next settings.Settings) func()
	// PruneLogsIfRetentionChanged prunes the session logs when the retention changed.
	PruneLogsIfRetentionChanged(old, next settings.Settings)
	KickGuards()
}

// ReloadAppliers is the root's registration. The order is the apply order:
// budget, chat, store commits, swap, then certs, config watch, log prune and
// the runner kick.
func ReloadAppliers(h ReloadHost) settings.Appliers {
	return settings.Appliers{
		Before: []settings.Applier{
			func(old, next settings.Settings) func() { h.ApplyLimits(old, next); return nil },
			h.ApplyChat,
			h.ApplyStoreCommits,
		},
		After: []settings.Applier{
			func(old, next settings.Settings) func() { h.ReissueCerts(old, next); return nil },
			h.ApplyWatchConfig,
			func(old, next settings.Settings) func() { h.PruneLogsIfRetentionChanged(old, next); return nil },
			func(settings.Settings, settings.Settings) func() { h.KickGuards(); return nil },
		},
	}
}
