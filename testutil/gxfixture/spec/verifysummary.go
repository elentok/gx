package spec

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// notesEpic is one queued ticket whose agent leaves a Verified entry and an
// iteration summary before it finishes.
func notesEpic(name string, opts servertest.TicketOpts) scenario {
	return scenario{
		Epics:     []epic{{project, name, []ticket{{"01", "first", opts}}}},
		Queue:     []string{project + ":" + name},
		Behaviour: map[string]string{project + ":" + name + "/01": agentfake.BehaviourNotes},
	}
}

var (
	// VerifyNotes: a verify ticket whose agent writes a "## Verified" entry for its
	// story and an iteration summary; gx adds its own summary entry at the end.
	VerifyNotes = notesEpic("verify-epic", servertest.TicketOpts{Type: "verify"})
	// SummaryNotes: an implement ticket whose agent writes an iteration summary, so
	// the ticket detail shows the "Latest summary" block once it finishes.
	SummaryNotes = notesEpic("summary-epic", servertest.TicketOpts{})
	// MapEvolve: a design map closed done, and the evolve one-off its completion
	// started, stamped on the map as evolve_ticket.
	MapEvolve = scenario{
		Epics: []epic{
			{project, "closed-map", nil},
			{project, tickets.OneOffsEpic, []ticket{
				{"01", "evolve-closed-map", servertest.TicketOpts{Type: "evolve", Unique: "evolve:" + project + "/closed-map"}},
			}},
		},
		EpicMeta: map[string]string{
			project + ":closed-map": "status: done\nkind: map\ncompleted_at: 2026-01-01T10:00:00Z\n" +
				"evolved_at: 2026-01-01T11:00:00Z\nevolve_ticket: " + project + ":" + tickets.OneOffsEpic + "/01\n",
			project + ":" + tickets.OneOffsEpic: "status: draft\n",
		},
	}
)
