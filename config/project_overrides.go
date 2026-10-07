package config

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// projectKeys is the whitelist of project.json keys: name/repo identify the
// project, the rest override config.json. A nested map whitelists the sub-keys
// of an object key; nil means the key is a leaf or free-form below.
var projectKeys = map[string]map[string]bool{
	"name":            nil,
	"repo":            nil,
	"vcs":             nil,
	"trunk":           nil,
	"landing":         nil,
	"auto-ff-merge":   nil,
	"max-agents":      nil,
	"agents":          nil,
	"skills":          nil,
	"notifications":   nil,
	"execution-queue": {"max-agents-per-epic": true},
}

// checkProjectKeys rejects any key outside the whitelist, naming it. Global-only
// keys are errors too: silently ignoring one would hide a misplaced setting.
func checkProjectKeys(data []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil // the typed parse reports malformed JSON
	}
	var bad []string
	for key, raw := range top {
		sub, ok := projectKeys[key]
		if !ok {
			bad = append(bad, key)
			continue
		}
		if sub == nil {
			continue
		}
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) != nil {
			continue
		}
		for k := range nested {
			if !sub[k] {
				bad = append(bad, key+"."+k)
			}
		}
	}
	if len(bad) == 0 {
		return nil
	}
	slices.Sort(bad)
	return fmt.Errorf("key not allowed in project.json: %s", strings.Join(bad, ", "))
}

// WithProject returns c with the project's overrides layered on top. c is a
// value, so the global config is untouched. An empty string or nil slice in an
// override means "not set" and keeps the global value.
func (c Config) WithProject(pf ProjectFile) Config {
	if pf.ExecutionQueue != nil && pf.ExecutionQueue.MaxAgentsPerEpic != nil {
		c.ExecutionQueue.MaxConcurrentTicketsPerEpic = clampExecutionQueueLimit(*pf.ExecutionQueue.MaxAgentsPerEpic)
	}
	if a := pf.Agents; a != nil {
		c.Agents.Claude = overlayAgent(c.Agents.Claude, a.Claude)
		c.Agents.Codex = overlayAgent(c.Agents.Codex, a.Codex)
	}
	if s := pf.Skills; s != nil {
		c.Skills.Implement = firstNonEmpty(s.Implement, c.Skills.Implement)
		if s.CodeReview != nil {
			c.Skills.CodeReview = s.CodeReview
		}
	}
	if n := pf.Notifications; n != nil {
		// Replaces the whole block: a destination is transport + target, so
		// merging per transport would mix one project's token with another's chat.
		c.Notifications = *n
	}
	return c
}

func overlayAgent(base, over AgentConfig) AgentConfig {
	base.Model = firstNonEmpty(over.Model, base.Model)
	base.Effort = firstNonEmpty(over.Effort, base.Effort)
	return base
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
