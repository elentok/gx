package spec

import (
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// ToolsScenario is three Working Claude sessions with tool calls in flight: one
// shell call, one shell, sub-agent and search together, and a design chat on a
// map with one shell call. A held Queue agent runs beside them.
func ToolsScenario() scenario {
	const mapAddr = "design-epic"
	inFlight := func(calls ...agentfake.ToolCall) *agentfake.Control {
		return &agentfake.Control{Action: agentfake.ActionToolUse, ToolCalls: calls}
	}
	shell := agentfake.ToolCall{Name: "Bash", Input: map[string]any{"command": "make test"}}
	sc := scenario{
		Epics:       []epic{{project, mapAddr, nil}},
		DesignSteps: map[string]string{project + ":" + mapAddr: string(tickets.StepResolveTickets)},
		Chat:        true,
		Sessions: []sessionSpec{
			{Project: project, Prompt: "hello", Control: inFlight(shell)},
			{Project: project, Prompt: "hello", Control: inFlight(
				shell,
				agentfake.ToolCall{Name: "Agent", Input: map[string]any{"description": "Survey the sessions code", "subagent_type": "Explore"}},
				agentfake.ToolCall{Name: "Grep", Input: map[string]any{"pattern": "InFlightTool"}},
			)},
			{Project: project, Prompt: "hello", Link: project + ":" + mapAddr, Control: inFlight(shell)},
		},
	}
	addQueueAgent(&sc)
	return sc
}
