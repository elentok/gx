package ticketwrite

import (
	"testing"

	"github.com/elentok/gx/ralphloop/agent"
)

func TestLaunchTier(t *testing.T) {
	for _, c := range []struct {
		name, typ string
		override  agent.Tier
		want      agent.Tier
	}{
		{"pr-text runs light", "pr-text", "", agent.TierLight},
		{"an override wins over the light default", "pr-text", agent.TierStrongest, agent.TierStrongest},
		{"implement keeps the worker tier", "implement", "", ""},
		{"an override applies to any type", "implement", agent.TierLight, agent.TierLight},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := launchTier(c.typ, c.override); got != c.want {
				t.Fatalf("launchTier(%q, %q) = %q, want %q", c.typ, c.override, got, c.want)
			}
		})
	}
}
