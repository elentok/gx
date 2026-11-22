package recovery

import (
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const (
	implementSkill   = "gx-implement"
	oneOffSkill      = "gx-one-off"
	evolveSkill      = "gx-evolve"
	investigateSkill = "gx-investigate"
)

// LaunchSkill is the skill an agent starts under. Prompt tickets (the
// one-offs) get one that keeps the TDD rules out of the way, and investigate
// and diagnose tickets run gx-investigate's unattended mode. Every other type
// is decided by agent.LaunchSkill.
func LaunchSkill(t tickets.Ticket) string {
	switch {
	case t.Type == string(schema.TypePrompt), t.Type == string(schema.TypeTrunkConflict):
		return oneOffSkill
	case t.Type == string(schema.TypeEvolve):
		return evolveSkill
	case t.Type == string(schema.TypeInvestigate), t.Type == string(schema.TypeDiagnose):
		return investigateSkill
	}
	return agent.LaunchSkill(t, implementSkill)
}

// LaunchPrompt is the first prompt of an agent: the skill command and the
// ticket address. A launch note never rides in it: a long one makes Claude
// Code take the typed keys as a paste (see the launcher's note file).
func LaunchPrompt(agentKind agent.AgentKind, skill, address string) string {
	return agent.SkillPrompt(agentKind, skill, address)
}
