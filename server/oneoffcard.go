package server

import (
	"cmp"
	"fmt"
	"github.com/elentok/gx/server/wire"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// oneOffCards lists the cards of e's one-off tickets, none when e holds none. A
// done or archived one-off gets a Finished card; capFinishedOneOffs trims those.
func (s *Server) oneOffCards(project string, e tickets.Epic, queue queueIndex) []wire.EpicListRow {
	if !IsOneOffEpic(e) {
		return nil
	}
	var rows []wire.EpicListRow
	for _, f := range finishedOneOffs(project, e) {
		row := oneOffRow(f.Address, f.Name, project)
		row.State, row.Section, row.CompletedAt = wire.EpicListFinished, wire.SectionFinished, f.CompletedAt
		setCardTime(&row, cardTimeSources{})
		rows = append(rows, row)
	}
	if filepath.Base(e.Path) == OneOffsArchivedEpic {
		return rows
	}
	root := rootRef{Project: project, Epic: e.Name}
	for _, t := range e.Tickets {
		addr := tickets.Address{Project: project, Epic: e.Name, ID: t.DisplayNumber()}
		row := oneOffRow(addr.String(), t.Title, project)
		if n, ok := s.ticketWaitOf(root, e, t); ok {
			row.State, row.Section = wire.EpicListPaused, wire.SectionNeedsYou
			row.StatePill, row.Reason, row.Actions = n.Pill, n.Reason, []wire.CardAction{n.Action}
			row.WaitingSince = cmp.Or(n.Since, t.ClaimedAt, lastActivity(e))
		} else if e.RenderedStatus(t) == tickets.StatusClaimed {
			row.State, row.Section = wire.EpicListRunning, wire.SectionImplementing
		} else if slot := queue.slot(addr.String()); slot > 0 {
			row.State, row.Section = wire.EpicListQueued, wire.SectionImplementing
			row.QueueSlot, row.StatusWords = slot, fmt.Sprintf("Queued #%d", slot)
		} else {
			continue
		}
		setCardTime(&row, cardTimeSources{startedAt: t.ClaimedAt, enqueuedAt: queue.enqueuedAt(addr.String())})
		rows = append(rows, row)
	}
	return rows
}

func oneOffRow(address, title, project string) wire.EpicListRow {
	return wire.EpicListRow{
		Address: address, Project: project, Name: title, Kind: wire.EpicKindLoop,
		Addresses: []string{address}, CardKind: wire.CardKindOneOff,
		StatusWords: "One-off", Actions: []wire.CardAction{},
	}
}

// capFinishedOneOffs drops the Finished one-off cards that are not among the
// newest limit Finished cards. Done epics always stay.
func capFinishedOneOffs(rows []wire.EpicListRow, limit int) []wire.EpicListRow {
	var finished []wire.EpicListRow
	for _, r := range rows {
		if r.Section == wire.SectionFinished {
			finished = append(finished, r)
		}
	}
	slices.SortStableFunc(finished, func(a, b wire.EpicListRow) int { return finishedAt(b).Compare(finishedAt(a)) })
	if len(finished) > limit {
		cut := map[string]bool{}
		for _, r := range finished[limit:] {
			cut[r.Address] = true
		}
		return slices.DeleteFunc(rows, func(r wire.EpicListRow) bool {
			return r.CardKind == wire.CardKindOneOff && r.Section == wire.SectionFinished && cut[r.Address]
		})
	}
	return rows
}

// followUpsCard is the one card of a project's follow-ups epic, absent when it
// has no open follow-up. The epic itself never gets a plain card.
func followUpsCard(project string, e tickets.Epic) (wire.EpicListRow, bool) {
	var bugs, ideas, other int
	for _, t := range e.Tickets {
		if t.Status != string(schema.StatusDraft) && t.Status != string(schema.StatusOpen) {
			continue
		}
		switch t.Category {
		case string(schema.CategoryBug):
			bugs++
		case string(schema.CategoryIdea):
			ideas++
		default:
			other++
		}
	}
	if bugs+ideas+other == 0 {
		return wire.EpicListRow{}, false
	}
	address := rootRef{Project: project, Epic: followUpsEpicName}.String()
	return wire.EpicListRow{
		Address: address, Project: project, Name: followUpsEpicName, Kind: wire.EpicKindLoop,
		State: wire.EpicListFollowUps, Addresses: []string{address}, CardKind: wire.CardKindFollowUps,
		Section: wire.SectionReadyToDesign, StatusWords: followUpWords(bugs, ideas, other),
		Actions: []wire.CardAction{{Kind: wire.CardActionTriage, Label: "Triage", Target: project}},
	}, true
}

// followUpWords reads "5 bugs, 2 ideas"; follow-ups with no category count as "other".
func followUpWords(bugs, ideas, other int) string {
	var parts []string
	for _, c := range []struct {
		n         int
		one, many string
	}{{bugs, "bug", "bugs"}, {ideas, "idea", "ideas"}, {other, "other", "other"}} {
		if c.n == 1 {
			parts = append(parts, fmt.Sprintf("1 %s", c.one))
		} else if c.n > 1 {
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.many))
		}
	}
	return strings.Join(parts, ", ")
}
