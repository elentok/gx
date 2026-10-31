package server

import (
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// HarnessLabel is what a ticket row shows as `harness · tier · model effort`.
// A waiting ticket carries the harness it will launch under and why in
// HarnessNote; a running one carries what its run launched under.
type HarnessLabel struct {
	// Harness is "claude" or "codex"; empty for a waiting ticket that follows the default.
	Harness     string `json:"harness,omitempty"`
	Tier        string `json:"tier,omitempty"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	HarnessNote string `json:"harness_note,omitempty"`
}

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

// harnessLabelFor is the label of the ticket at addr with frontmatter agent
// agent and the given rendered status. Only open, blocked and claimed tickets
// carry one.
func (s *Server) harnessLabelFor(addr tickets.Address, agent string, status string) HarnessLabel {
	switch status {
	case tickets.StatusClaimed.Word():
		run, ok := s.registry.runOf(addr.String())
		if !ok {
			return HarnessLabel{}
		}
		kind := ralphloop.AgentKind(run.Agent)
		model, effort := s.runAgentConfig(addr.Project, kind, run)
		return HarnessLabel{Harness: run.Agent, Tier: string(ralphloop.TierWorker), Model: model, Effort: effort}
	case tickets.StatusOpen.Word(), tickets.StatusBlocked.Word():
		r := s.resolveTicket(addr, tickets.Ticket{Agent: agent})
		switch r.Source {
		case HarnessFromFrontmatter, HarnessFromQueueItem:
			return HarnessLabel{Harness: string(r.Harness), HarnessNote: harnessNotePinned}
		case HarnessFromEarlierRun:
			return HarnessLabel{Harness: string(r.Harness), HarnessNote: "keeps " + harnessDisplayName(r.Harness) + " (ran before)"}
		}
		return HarnessLabel{HarnessNote: harnessNoteFollowsDefault}
	}
	return HarnessLabel{}
}
