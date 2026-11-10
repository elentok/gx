package spec

import "github.com/elentok/gx/tickets"

// InterruptedScenario is a Claude and a Codex session, each as a Chat session
// and as a Design session of the design-epic map, whose chat holds one
// interrupted turn.
func InterruptedScenario() scenario {
	const mapAddr = project + ":design-epic"
	return scenario{
		Epics:       []epic{{project, "design-epic", nil}},
		DesignSteps: map[string]string{mapAddr: string(tickets.StepResolveTickets)},
		Chat:        true,
		Codex:       true,
		Sessions: []sessionSpec{
			{Project: project, Prompt: "hello", Interrupted: true},
			{Project: project, Prompt: "hello", Harness: "codex", Interrupted: true},
			{Project: project, Prompt: "hello", Link: mapAddr, Interrupted: true},
			{Project: project, Prompt: "hello", Harness: "codex", Link: mapAddr, Interrupted: true},
		},
	}
}
