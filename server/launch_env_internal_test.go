package server

import (
	"slices"
	"testing"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

func TestAgentEnv_SetsTheRunsAgent(t *testing.T) {
	t.Parallel()
	for _, agent := range []ralphloop.AgentKind{ralphloop.AgentClaude, ralphloop.AgentCodex} {
		env := agentEnv([]string{"A=1"}, tickets.Ticket{}, tickets.Address{}, agent)
		if want := AgentEnv + "=" + string(agent); !slices.Contains(env, want) {
			t.Errorf("agentEnv for %s = %v, want it to hold %q", agent, env, want)
		}
		if !slices.Contains(env, "A=1") {
			t.Errorf("agentEnv for %s = %v, lost the tab env", agent, env)
		}
	}
}
