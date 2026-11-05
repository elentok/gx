package tickettest

import (
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

// Muted is a ticket's mute list with one active record.
var Muted = []schema.MuteRecord{{EventType: "notification-storm"}}

// DetailWith is a detail whose actions carry the table's label and tooltip, as
// the server sends them.
func DetailWith(actions ...string) wire.TicketDetail {
	d := wire.TicketDetail{Actions: actions, ActionInfo: map[string]wire.ActionInfo{}}
	for _, a := range actions {
		v, ok := schema.VerbOfWord(a)
		if !ok {
			continue
		}
		d.ActionInfo[a] = wire.ActionInfo{Label: v.Label(), Tooltip: v.Tooltip()}
	}
	return d
}
