package tickets

import (
	"encoding/json"
	"io"

	"github.com/elentok/gx/cmd/ticketref"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
)

// runTicketsLog is file-only: the log sits beside the ticket file, so it needs
// no server route.
func runTicketsLog(dirs config.Dirs, cwd, addr, eventType string, w io.Writer) error {
	_, t, err := ticketref.FindTicket(dirs, cwd, addr)
	if err != nil {
		return err
	}
	events, err := tickets.ReadLog(t.Path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	for _, ev := range events {
		if eventType != "" && ev.Type != eventType {
			continue
		}
		if err := enc.Encode(ev); err != nil {
			return err
		}
	}
	return nil
}
