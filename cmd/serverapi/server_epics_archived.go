package serverapi

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

// newServerEpicsArchivedCmd lists the archived epics and Designs.
func newServerEpicsArchivedCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "archived",
		Short: "list the archived epics and Designs, newest first",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.ArchivedEpicList, error) {
					return cl.Reads.ArchivedEpics(ctx)
				},
				printArchivedList)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

// newServerEpicsArchivedDetailCmd shows one archived epic by its archive address.
func newServerEpicsArchivedDetailCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "archived-detail <project:epic>",
		Short: "show an archived epic's ticket rows, by the address `archived` lists",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.EpicDetail, error) {
					return cl.Reads.ArchivedEpicDetail(ctx, args[0])
				},
				printEpicDetail)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

// printArchivedList writes one line per row, in the server's order.
func printArchivedList(w io.Writer, l wire.ArchivedEpicList) error {
	for _, r := range l.Epics {
		at := "-"
		if !r.ArchivedAt.IsZero() {
			at = r.ArchivedAt.Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Address, r.Kind, at, r.Workspace, r.Title); err != nil {
			return err
		}
	}
	return nil
}
