package tabapi

import (
	"strconv"
	"strings"

	"github.com/elentok/gx/server/wire"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/viewmodel"
)

// EpicsFromViewModel groups the view model's tickets (already in snapshot
// order) into the epics the sidebar renders. An epic is keyed by project and
// name, since two projects can share an epic name.
func EpicsFromViewModel(vm viewmodel.State, scope viewmodel.Scope) []gxtickets.Epic {
	var epics []gxtickets.Epic
	byName := map[string]int{}
	for _, info := range vm.ScopedTickets(scope) {
		epic, id, ok := gxtickets.SplitTrailerValue(info.Address)
		if !ok {
			continue
		}
		project, _, _ := strings.Cut(info.Address, ":")
		name := project + ":" + epic
		i, seen := byName[name]
		if !seen {
			i = len(epics)
			byName[name] = i
			epics = append(epics, gxtickets.Epic{
				Name: name, Path: name, MergedAt: info.EpicMergedAt, IsMap: info.EpicIsMap,
				Status: info.EpicStatus, MergeTarget: info.EpicMergeTarget,
			})
		}
		epics[i].Tickets = append(epics[i].Tickets, TicketFromInfo(info, id))
	}
	return epics
}

// TicketFromInfo converts a wire ticket into the tracker's ticket type.
func TicketFromInfo(info wire.TicketInfo, id string) gxtickets.Ticket {
	// The number is the leading digits only: a multi-level fork id like
	// "27a1" ends in a digit, so trimming trailing letters would leave it
	// unparseable and every parent/blocker lookup for it would miss.
	digits := strings.IndexFunc(id, func(r rune) bool { return r < '0' || r > '9' })
	if digits < 0 {
		digits = len(id)
	}
	n, _ := strconv.Atoi(id[:digits])
	t := gxtickets.Ticket{
		Number:     n,
		Identifier: id,
		Title:      info.Title,
		Path:       info.Address,
		File:       info.File,
		Type:       info.Type,
		BlockedBy:  info.BlockedBy,
		Status:     info.Status,

		ExternalBlockers: info.ExternalBlockers,
		WidenedBlockers:  info.WidenedBlockers,

		ActualContextWindow:   info.ActualContextWindow,
		ExpectedContextWindow: info.ExpectedContextWindow,
		ElapsedTime:           info.ElapsedTime,
		ActualCost:            info.ActualCost,
		HadCodex:              info.HadCodex,
		Compactions:           info.Compactions,

		Category:   info.Category,
		ExportedAt: info.ExportedAt,
		Mutes:      info.Mutes,
	}
	if rendered, ok := gxtickets.ParseRenderedStatus(info.RenderedStatus); ok {
		t.Rendered = &rendered
	}
	if info.Parent != "" {
		_, parent, _ := gxtickets.SplitTrailerValue(info.Parent)
		t.Parent = &parent
	}
	return t
}
