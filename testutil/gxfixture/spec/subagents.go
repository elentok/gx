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
	"stopped-no-handback", "background-returned", "nested", "starting", "finished-no-handback", "string-background", "parent-gone", "exited",
	"codex-starting", "codex-child", "codex-null-role", "codex-compressed", "codex-nested",
}

// SubagentsScenario is the subagents scenario in the given shape: one Working
// Claude Design session of design-epic whose transcript and subagent files hold
// the shape's subagents (see bgtx). False for a name that is not a shape.
func SubagentsScenario(shape string) (Scenario, bool) {
	const mapAddr = project + ":design-epic"
	if sub, ok := bgtx.CodexSubagentShapes[shape]; ok {
		return Scenario{
			Epics: []Epic{{Project: project, Name: "design-epic"}},
			Chat:  true,
			Codex: true,
			Sessions: []SessionSpec{bgCodexSession(shape, 1, sub.Build, func(s *bgtx.Seed) {
				// The wait is for the one task row: a spawn_agent call has no tools entry to name.
				s.Version, s.Calls = bgtx.CodexSubVersion, sub.Calls
			})},
		}, true
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
	return Scenario{
		Epics:       []Epic{{Project: project, Name: "design-epic"}},
		DesignSteps: map[string]string{mapAddr: string(tickets.StepResolveTickets)},
		Chat:        true,
		Sessions:    []SessionSpec{session},
	}, true
}
