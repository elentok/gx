package serverapi

import (
	"context"
	"fmt"
	"io"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// newServerBacklogCmd lists the areas the Backlog page shows.
func newServerBacklogCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "backlog",
		Short: "list each project's areas with their open ticket counts and unfinished Start Design moves",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.BacklogList, error) {
					return cl.Reads.Backlog(ctx)
				},
				printBacklog)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

// printBacklog writes one line per area: address, open count, then the
// unfinished move's map and count to move, or "-".
func printBacklog(w io.Writer, l wire.BacklogList) error {
	for _, p := range l.Projects {
		for _, a := range p.Areas {
			move := "-"
			if a.Unfinished != nil {
				move = fmt.Sprintf("%s (%d to move)", a.Unfinished.Map, a.Unfinished.ToMove)
			}
			if _, err := fmt.Fprintf(w, "%s\t%d\t%s\n", a.Address, a.Open, move); err != nil {
				return err
			}
		}
	}
	return nil
}
