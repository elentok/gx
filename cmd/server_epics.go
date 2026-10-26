package cmd

import (
	"cmp"
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
	var listJSON, allFinished bool
	cmd := &cobra.Command{
		Use:   "epics",
		Short: "list every epic the Epics page shows, or act on a whole epic through the gx server",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return serverRead(c.Context(), listJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (server.EpicList, error) {
					return cl.Epics(ctx)
				},
				func(w io.Writer, l server.EpicList) error { return printEpicList(w, l, allFinished) })
		},
	}
	cmd.Flags().BoolVar(&listJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.Flags().BoolVar(&allFinished, "all-finished", false, "print every Finished card, not only the first server.recently-finished")
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

	stashMerge := &cobra.Command{
		Use:   "stash-merge <project:epic>",
		Short: "merge an epic paused on a dirty trunk: set the clashing files aside, merge, restore them with conflict markers",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, ok := tickets.ParseEpicAddress(args[0]); !ok {
				return fmt.Errorf("%s is not an epic address (want project:epic)", args[0])
			}
			return serverQueueWrite(c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return cl.EpicStashMerge(ctx, args[0])
			})
		},
	}
	stashMerge.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(stashMerge)

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
func printEpicRow(w io.Writer, address, kind, state string, done, total int, queueable bool, extra ...string) error {
	line := fmt.Sprintf("%s\t%s\t%s\t%d/%d\tqueueable=%t", address, kind, state, done, total, queueable)
	for _, col := range extra {
		line += "\t" + col
	}
	_, err := fmt.Fprintln(w, line)
	return err
}

// printEpicList writes one line per card. Finished cards come newest first, and
// only the first l.RecentlyFinished print unless allFinished is set.
func printEpicList(w io.Writer, l server.EpicList, allFinished bool) error {
	finished := 0
	for _, e := range l.Epics {
		if e.Section == server.SectionFinished {
			finished++
			if !allFinished && finished > l.RecentlyFinished {
				continue
			}
		}
		// A joined card prints the active part's address, the first of Addresses.
		address := e.Address
		if len(e.Addresses) > 0 {
			address = e.Addresses[0]
		}
		if err := printEpicRow(w, address, string(cmp.Or(e.CardKind, server.CardKind(e.Kind))), string(e.State), e.Done, e.Total, e.Queueable, string(e.Section)); err != nil {
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
