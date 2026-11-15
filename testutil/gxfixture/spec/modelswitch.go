package spec

import (
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// Models the model-switch scenario's tiers name. Claude's worker tier is the
// lagging alias (it resolves to a model older than claude-fable-5-2); Codex's
// is the outdated model (its list row names an upgrade).
const (
	LaggingClaudeAlias = "fable"
	OutdatedCodexModel = "gpt-5.4"
)

const modelSwitchAgents = `,"agents":{` +
	`"claude":{"model":"` + LaggingClaudeAlias + `","effort":"medium","light":{"model":"haiku"},"strongest":{"model":"opus","effort":"high"}},` +
	`"codex":{"model":"` + OutdatedCodexModel + `","effort":"medium","light":{"model":"gpt-6-mini"},"strongest":{"model":"gpt-5.5","effort":"high"}}}`

// modelSwitchModels is the one list both fakes answer the model query with:
// the Claude rows, then the Codex rows the Claude list does not already have.
func modelSwitchModels() []agentfake.ProbeModel {
	models := agentfake.DefaultClaudeModels()
	seen := map[string]bool{}
	for _, m := range models {
		seen[m.Value] = true
	}
	for _, m := range agentfake.DefaultCodexModels() {
		if !seen[m.Value] {
			models = append(models, m)
		}
	}
	return models
}

// ModelSwitchScenario is the world of the model-switch flows: in demo, whose
// tiers name models of the fake lists, an idle Codex chat with history on the
// outdated model, an idle Claude chat with history on the lagging alias, a
// chat with no turns and a design session. Flows reach the busy and stopped
// states themselves.
func ModelSwitchScenario() scenario {
	const designMap = "switch-design"
	return scenario{
		Codex:        true,
		Chat:         true,
		ModelFakes:   true,
		Epics:        []epic{{project, designMap, nil}},
		DesignSteps:  map[string]string{project + ":" + designMap: string(tickets.StepResolveTickets)},
		OpenDesigns:  []string{project + ":" + designMap},
		ProjectExtra: map[string]string{project: modelSwitchAgents},
		Commands:     &agentfake.ProbeConfig{Models: modelSwitchModels()},
		Sessions: []sessionSpec{
			{Project: project, Harness: "codex", Prompt: "hello", Read: true},
			{Project: project, Prompt: "hello", Read: true},
			{Project: project},
		},
	}
}
