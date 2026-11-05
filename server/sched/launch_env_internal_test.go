package sched

import (
	"slices"
	"testing"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/tickets"
)

func TestAgentEnv_SetsTheRunsAgent(t *testing.T) {
	t.Parallel()
	for _, agent := range []agent.AgentKind{agent.AgentClaude, agent.AgentCodex} {
		env := agentEnv([]string{"A=1"}, tickets.Ticket{}, tickets.Address{}, agent)
		if want := projects.AgentEnv + "=" + string(agent); !slices.Contains(env, want) {
			t.Errorf("agentEnv for %s = %v, want it to hold %q", agent, env, want)
		}
		if !slices.Contains(env, "A=1") {
			t.Errorf("agentEnv for %s = %v, lost the tab env", agent, env)
		}
	}
}
