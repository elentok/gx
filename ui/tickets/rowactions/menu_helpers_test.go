package rowactions

import (
	"slices"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

var (
	muted        = tickettest.Muted
	detailWith   = tickettest.DetailWith
	resumeLabel  = schema.VerbResume.Label()
	answerDetail = detailWith(wire.ActionAnswer, wire.ActionResume)
)

func itemByLabel(t *testing.T, items []contextmenu.Item[TicketTarget], label string) contextmenu.Item[TicketTarget] {
	t.Helper()
	i := slices.IndexFunc(items, func(it contextmenu.Item[TicketTarget]) bool { return it.Label == label })
	if i < 0 {
		labels := make([]string, len(items))
		for j, it := range items {
			labels[j] = it.Group.String() + "/" + it.Label
		}
		t.Fatalf("no %q item in %v", label, labels)
	}
	return items[i]
}

func hasItem(items []contextmenu.Item[TicketTarget], label string) bool {
	return slices.ContainsFunc(items, func(it contextmenu.Item[TicketTarget]) bool { return it.Label == label })
}
