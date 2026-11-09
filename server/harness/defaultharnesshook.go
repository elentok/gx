package harness

import (
	"encoding/json"
	"slices"
	"sync"

	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/settingsapi"
)

var defaultHarnessPath = registry.Key("agents", "default-harness")

// switchOutcome is what one Default harness write changed.
type switchOutcome struct {
	before, after    agent.AgentKind
	waiting, running int
	notice           string
	reloadRefused    bool
	reloadReason     string
	reloadMsg        string
}

// settingsHook is the settings writer's hook for agents.default-harness: it
// refuses a harness missing from PATH before the write and reports the switch
// after it. The writer's lock serializes Before and After.
type settingsHook struct {
	h *Harness

	mu     sync.Mutex
	before agent.AgentKind
	last   switchOutcome
}

func (k *settingsHook) change(changes []settingsapi.Change) (settingsapi.Change, bool) {
	i := slices.IndexFunc(changes, func(c settingsapi.Change) bool { return slices.Equal(c.Key, defaultHarnessPath) })
	if i < 0 {
		return settingsapi.Change{}, false
	}
	return changes[i], true
}

func (k *settingsHook) Before(t settingsapi.Target, changes []settingsapi.Change) error {
	c, ok := k.change(changes)
	if !ok {
		return nil
	}
	k.mu.Lock()
	k.before = k.h.effectiveDefault(t.Project)
	// A write that never reaches After (removing a key from a missing file) changes nothing.
	k.last = switchOutcome{before: k.before, after: k.before}
	k.mu.Unlock()
	if c.Remove {
		return nil
	}
	var name string
	if err := json.Unmarshal(c.Value, &name); err != nil {
		return nil // Apply refuses a bad value
	}
	if program, missing := k.h.MissingProgram(name); missing {
		return &settingsapi.Refusal{Keys: []registry.KeyPath{defaultHarnessPath}, Reason: program + " is not on PATH"}
	}
	return nil
}

func (k *settingsHook) After(t settingsapi.Target, changes []settingsapi.Change) string {
	if _, ok := k.change(changes); !ok {
		return ""
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	out := switchOutcome{before: k.before}
	if t.Project == "" {
		if rr := k.h.settings.Reload(settings.TriggerCommand); rr.Refused {
			out.reloadRefused, out.reloadReason, out.reloadMsg = true, rr.Reason, rr.Message
			k.last = out
			return ""
		}
	}
	out.after = k.h.effectiveDefault(t.Project)
	if out.before != out.after {
		out.waiting, out.running = k.h.switchCounts(t.Project, out.after)
		out.notice = defaultHarnessNotice(t.Project, out.after, out.waiting, out.running)
	}
	k.last = out
	return out.notice
}

func (k *settingsHook) outcome() switchOutcome {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.last
}
