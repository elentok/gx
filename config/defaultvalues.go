package config

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/nativerunner"
)

// hiddenDefaults are the keys the file accepts but Config does not marshal
// (`json:"-"`), each read from Default() as the value the file would hold.
var hiddenDefaults = map[string]func(Config) any{
	"runner-wait":          func(c Config) any { return durationText(c.RunnerWait) },
	"recovery.notify-hold": func(c Config) any { return durationText(c.Recovery.NotifyHold) },
	"pr.no-checks-grace":   func(c Config) any { return durationText(c.PR.NoChecksGrace) },
	// Zero means the runner's own default, which is what the page shows.
	"server.log-retention": func(c Config) any {
		if c.Server.LogRetention == 0 {
			return durationText(nativerunner.DefaultLogRetention)
		}
		return durationText(c.Server.LogRetention)
	},
	"cpu-watchdog.sustain":                func(c Config) any { return durationText(c.CPUWatchdog.Sustain) },
	"cpu-watchdog.cooldown":               func(c Config) any { return durationText(c.CPUWatchdog.Cooldown) },
	"watchdog.interval":                   func(c Config) any { return durationText(c.Watchdog.Interval) },
	"execution-queue.spin-window":         func(c Config) any { return durationText(c.ExecutionQueue.SpinWindow) },
	"execution-queue.waiting-slot-grace":  func(c Config) any { return durationText(c.ExecutionQueue.WaitingSlotGrace) },
	"execution-queue.stalled-nudge-delay": func(c Config) any { return durationText(c.ExecutionQueue.StalledNudgeDelay) },
}

// durationText prints a duration in its shortest form: 5m, not 5m0s.
func durationText(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// DefaultValue is the default of a registry key path, shaped as the config
// file would hold it (durations as "5m", sizes as "20G", lists as arrays). It
// reports false for secrets and for keys with no default.
func DefaultValue(path registry.KeyPath) (any, bool) {
	if e, ok := registry.Default().Lookup(path); ok && e.WriteOnly() {
		return nil, false
	}
	if node, ok := valueAt(Default(), path); ok {
		return node, true
	}
	return emptyDefault(path)
}

// ValueAt is the value of a registry key path in cfg, shaped as the config
// file would hold it; nil when cfg holds none. Comparing two configs through
// it needs no per-key code.
func ValueAt(cfg Config, path registry.KeyPath) any {
	node, _ := valueAt(cfg, path)
	return node
}

// valueAt reports false when cfg holds no value at path.
func valueAt(cfg Config, path registry.KeyPath) (any, bool) {
	if f, ok := hiddenDefaults[path.String()]; ok {
		return f(cfg), true
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, false
	}
	var node any
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, false
	}
	for _, seg := range path {
		m, ok := node.(map[string]any)
		if !ok {
			return nil, false
		}
		if node, ok = m[seg]; !ok {
			return nil, false
		}
	}
	return node, node != nil
}

// emptyDefault is the default of a key the marshalled Config leaves out or
// holds as null (omitempty strings, nil lists): empty text or an empty list.
func emptyDefault(path registry.KeyPath) (any, bool) {
	e, ok := registry.Default().Lookup(path)
	if !ok {
		return nil, false
	}
	switch e.Type {
	case registry.TypeStringList, registry.TypeNumberList:
		return []any{}, true
	case registry.TypeString:
		return "", true
	}
	return nil, false
}
