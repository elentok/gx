package spec

import (
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/bgtx"
	"github.com/elentok/gx/tickets"
)

// SubagentsShapes are the --shape names of the subagents scenario; the first is
// the default and holds the mock states of the spec.
var SubagentsShapes = []string{
	"claude", "foreground-running", "finished-handback", "file-gone",
	"stopped-no-handback", "background-returned", "nested", "starting", "finished-no-handback", "string-background", "file-call", "parent-gone", "exited",
	"codex-starting", "codex-child", "codex-null-role", "codex-compressed", "codex-nested", "codex-running-call", "codex-old-call",
	"card-two-running", "card-one-subagent", "card-no-tasks",
}

// SubagentsScenario is the subagents scenario in the given shape: one session
// whose transcript and subagent files hold the shape's subagents (see bgtx). A
// Claude shape is a Working Design session of design-epic; a Codex shape is a
// plain session unless it is Linked. False for a name that is not a shape.
func SubagentsScenario(shape string) (Scenario, bool) {
	const mapAddr = project + ":design-epic"
	scenario := Scenario{Epics: []Epic{{Project: project, Name: "design-epic"}}, Chat: true}
	designSteps := map[string]string{mapAddr: string(tickets.StepResolveTickets)}
	if sub, ok := bgtx.CodexSubagentShapes[shape]; ok {
		// The wait is for the task rows: a spawn_agent call has no tools entry to name.
		session := bgCodexSession(shape, sub.Tasks, sub.Build, func(s *bgtx.Seed) {
			s.Version, s.Calls = sub.Version, sub.Calls
		})
		if sub.Linked {
			session.Link, scenario.DesignSteps = mapAddr, designSteps
		}
		scenario.Codex, scenario.Sessions = true, []SessionSpec{session}
		return scenario, true
	}
	sub, ok := bgtx.SubagentShapes[shape]
	if !ok {
		return Scenario{}, false
	}
	session := bgClaudeSession(mapAddr, shape, agentfake.ChatSlowMarker+" subagents", 0, bgSeed(sub.Build, func(s *bgtx.Seed) {
		s.Ready, s.Calls = sub.Ready, sub.Calls
	}))
	session.NoTranscript = shape == "parent-gone"
	session.Crash = shape == "exited"
	scenario.DesignSteps, scenario.Sessions = designSteps, []SessionSpec{session}
	return scenario, true
}
