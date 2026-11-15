package sessions

import (
	"time"

	"github.com/elentok/gx/modelrules"
	"github.com/elentok/gx/ralphloop/agent"
)

// RecordSwitch applies a successful hand switch to meta with the session's
// harness, project and tier as configured now. It does not write the file: the
// caller does once the new agent is alive.
func (mg *Manager) RecordSwitch(meta *Meta, model, effort string) {
	tierModel, _ := agent.ResolveTier(mg.projects.AgentsConfig(meta.Project), agent.AgentFor(meta.Harness), meta.Tier)
	list, _ := mg.chat.Models().Entries(meta.Harness)
	RecordSwitch(meta, model, effort, tierModel, list, time.Now())
}

// RecordSwitch sets the new model and effort and keeps the tier. The by-hand
// flag is set unless model is tierModel, the model configured for the tier; a
// Claude alias equals its resolved id in list, and a session with no tier
// always keeps the flag set. The rollback target is the record as it stands,
// except that a switch still unconfirmed keeps its earlier target, so a second
// switch rolls back to the last confirmed record.
func RecordSwitch(meta *Meta, model, effort, tierModel string, list []modelrules.Entry, now time.Time) {
	if !meta.SwitchUnconfirmed {
		meta.SwitchPrev = &SwitchTarget{Model: meta.Model, Effort: meta.Effort, ByHand: meta.SwitchedByHand}
	}
	meta.SwitchUnconfirmed = true
	meta.SwitchedAt = now
	meta.Model, meta.Effort = model, effort
	meta.SwitchedByHand = meta.Tier == "" || !sameModel(list, tierModel, model)
}

// sameModel reports whether two configured models are one, comparing a launch
// value with a resolved id through list.
func sameModel(list []modelrules.Entry, a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a == b || resolvedModelID(list, a) == resolvedModelID(list, b)
}

// resolvedModelID is the resolved id of a launch value, or the value itself
// when the list does not know it.
func resolvedModelID(list []modelrules.Entry, value string) string {
	for _, e := range list {
		if e.Value == value && e.ID != "" {
			return e.ID
		}
	}
	return value
}
