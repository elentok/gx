package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/spf13/cobra"
)

// newServerEpicsCmd lists the epics and holds the verbs that take an epic
// address; every `gx server tickets` verb takes a ticket address.
func newServerEpicsCmd() *cobra.Command {
	var listJSON bool
	cmd := &cobra.Command{
		Use:   "epics",
		Short: "list every epic the Epics page shows, or act on a whole epic through the gx server",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return serverRead(c.Context(), listJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (server.EpicList, error) {
					return cl.Epics(ctx)
				},
				printEpicList)
		},
	}
	cmd.Flags().BoolVar(&listJSON, "json", false, "emit structured JSON instead of human-readable text")
	var jsonOut bool
	merge := &cobra.Command{
		Use:   "merge <project:epic>",
		Short: "merge a complete epic now: merge it directly, or fork a merge ticket when it is not a fast-forward",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, ok := tickets.ParseEpicAddress(args[0]); !ok {
				return fmt.Errorf("%s is not an epic address (want project:epic)", args[0])
			}
			return serverQueueWrite(c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return cl.EpicMerge(ctx, args[0])
			})
		},
	}
	merge.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(merge)

	var detailJSON bool
	detail := &cobra.Command{
		Use:   "detail <project:epic>",
		Short: "show an epic's state, progress and ticket rows",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return serverRead(c.Context(), detailJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (server.EpicDetail, error) {
					return cl.EpicDetail(ctx, args[0])
				},
				printEpicDetail)
		},
	}
	detail.Flags().BoolVar(&detailJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(detail)
	return cmd
}

// printEpicRow writes the one-line epic summary shared by the list and detail verbs.
func printEpicRow(w io.Writer, address, kind, state string, done, total int, queueable bool) error {
	_, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d/%d\tqueueable=%t\n", address, kind, state, done, total, queueable)
	return err
}

func printEpicList(w io.Writer, l server.EpicList) error {
	for _, e := range l.Epics {
		if err := printEpicRow(w, e.Address, string(e.Kind), string(e.State), e.Done, e.Total, e.Queueable); err != nil {
			return err
		}
	}
	return nil
}

func printEpicDetail(w io.Writer, d server.EpicDetail) error {
	if err := printEpicRow(w, d.Address, string(d.Kind), string(d.State), d.Done, d.Total, d.Queueable); err != nil {
		return err
	}
	for _, r := range d.Rows {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", r.Address, r.RenderedStatus, r.Title); err != nil {
			return err
		}
	}
	return nil
}
