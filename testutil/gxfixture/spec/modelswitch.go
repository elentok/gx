package spec

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// Models the model-switch scenario's tiers name. Claude's worker tier is the
// lagging alias (it resolves to a model older than claude-fable-5-2); Codex's
// is the outdated model (its list row names an upgrade).
const (
	LaggingClaudeAlias = "fable"
	OutdatedCodexModel = "gpt-5.4-codex"
)

const modelSwitchAgents = `,"agents":{` +
	`"claude":{"model":"` + LaggingClaudeAlias + `","effort":"medium","light":{"model":"haiku"},"strongest":{"model":"opus","effort":"high"}},` +
	`"codex":{"model":"` + OutdatedCodexModel + `","effort":"medium","light":{"model":"gpt-6-mini"},"strongest":{"model":"gpt-5.5-codex","effort":"high"}}}`

// modelSwitchModels is the one config both fakes read: each answers its model
// query with its own harness's rows only.
func modelSwitchModels() []agentfake.ProbeModel {
	var models []agentfake.ProbeModel
	for _, m := range agentfake.DefaultClaudeModels() {
		m.Harness = "claude"
		models = append(models, m)
	}
	for _, m := range agentfake.DefaultCodexModels() {
		m.Harness = "codex"
		models = append(models, m)
	}
	return models
}

// ModelSwitchScenario is the world of the model-switch flows: in demo, whose
// tiers name models of the fake lists, an idle Codex chat with history on the
// outdated model, an idle Claude chat with history on the lagging alias, a
// chat with no turns, a Working Claude chat on the lagging alias and a design
// session. Flows reach the stopped state and the held switch themselves.
func ModelSwitchScenario() scenario {
	const designMap = "switch-design"
	return scenario{
		Codex:      true,
		Chat:       true,
		ModelFakes: true,
		// The impl epic is what the auto-queue countdown queues once the design reaches its Branch step.
		Epics: []epic{
			{project, designMap, nil},
			{project, designMap + "-impl", []ticket{{"01", "build", servertest.TicketOpts{}}}},
		},
		DesignSteps:  map[string]string{project + ":" + designMap: string(tickets.StepResolveTickets)},
		OpenDesigns:  []string{project + ":" + designMap},
		ProjectExtra: map[string]string{project: modelSwitchAgents},
		Commands:     &agentfake.ProbeConfig{Models: modelSwitchModels()},
		Sessions: []sessionSpec{
			{Project: project, Harness: "codex", Prompt: "hello", Read: true},
			{Project: project, Prompt: "hello", Read: true},
			{Project: project},
			// Last, so the flows that take the first Claude chats are unaffected. It is
			// outdated (lagging alias) and Working, for the busy popover note.
			{Project: project, Prompt: agentfake.ChatSlowMarker + " a long turn"},
		},
	}
}
