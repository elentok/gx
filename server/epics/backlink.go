package epics

import (
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/initiative"
)

// backLinkOf returns the Milestone back-link of a Design or its -impl epic.
// all holds the project's live and archived epics. An epic whose link does not
// resolve gets the zero value.
func backLinkOf(project string, e tickets.Epic, all []tickets.Epic) wire.BackLink {
	bl := initiative.BackLinkFor(e, all)
	if bl.Milestone == 0 {
		return wire.BackLink{}
	}
	return wire.BackLink{
		Milestone:         bl.Milestone,
		Initiative:        bl.Initiative,
		InitiativeAddress: queue.RootRef{Project: project, Epic: bl.Initiative}.String(),
	}
}

// withArchived returns live followed by the project's archived epics.
func (s *Epics) withArchived(project string, live []tickets.Epic) []tickets.Epic {
	archived, err := s.archivedOf(project)
	if err != nil {
		s.Log.Warn("back-link: read archived epics", "project", project, "err", err)
	}
	return append(append([]tickets.Epic{}, live...), archived...)
}
