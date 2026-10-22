package cmd

import (
	"encoding/json"
	"io"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/tickets"
)

func newTicketsLogCmd(d deps) *cobra.Command {
	var eventType string
	cmd := &cobra.Command{
		Use:   "log <addr>",
		Short: "print a ticket's raw log events, one JSON object per line",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsLog(cwd, args[0], eventType, c.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&eventType, "type", "", "only events of this type, e.g. pause.question")
	return cmd
}

// runTicketsLog is file-only: the log sits beside the ticket file, so it needs
// no server route.
func runTicketsLog(cwd, addr, eventType string, w io.Writer) error {
	_, t, err := findTicket(cwd, addr)
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
