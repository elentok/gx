package harness

import (
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

const (
	harnessNoteFollowsDefault = "follows default"
	harnessNotePinned         = "pinned"
)

func harnessDisplayName(h ralphloop.AgentKind) string {
	if h == ralphloop.AgentCodex {
		return "Codex"
	}
	return "Claude"
}

// LabelFor is the label of the ticket at addr with frontmatter agent
// agent and the given rendered status. Only open, blocked and claimed tickets
// carry one.
func (h *Harness) LabelFor(addr tickets.Address, agent string, status string) wire.HarnessLabel {
	switch status {
	case tickets.StatusClaimed.Word():
		run, ok := h.registry.RunOf(addr.String())
		if !ok {
			return wire.HarnessLabel{}
		}
		kind := ralphloop.AgentKind(run.Agent)
		model, effort := h.projects.RunAgentConfig(addr.Project, kind, run)
		return wire.HarnessLabel{Harness: run.Agent, Tier: string(ralphloop.TierWorker), Model: model, Effort: effort}
	case tickets.StatusOpen.Word(), tickets.StatusBlocked.Word():
		r := h.ResolveTicket(addr, tickets.Ticket{Agent: agent})
		switch r.Source {
		case HarnessFromFrontmatter, HarnessFromQueueItem:
			return wire.HarnessLabel{Harness: string(r.Harness), HarnessNote: harnessNotePinned}
		case HarnessFromEarlierRun:
			return wire.HarnessLabel{Harness: string(r.Harness), HarnessNote: "keeps " + harnessDisplayName(r.Harness) + " (ran before)"}
		}
		return wire.HarnessLabel{HarnessNote: harnessNoteFollowsDefault}
	}
	return wire.HarnessLabel{}
}
