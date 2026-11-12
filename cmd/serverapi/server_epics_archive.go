package serverapi

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

// newServerEpicsArchiveCmd archives finished loop epics; yes confirms stopping their live Sessions.
func newServerEpicsArchiveCmd(d Deps) *cobra.Command {
	var jsonOut, yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "archive <project:epic>...",
		Short: "archive finished loop epics into the project's .archive; their live sessions stop",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runTargets(c, d, jsonOut, args, func(ctx context.Context, cl *apiclient.Client) (wire.TargetsResult, error) {
				return cl.Admin.ArchiveEpics(ctx, args, yes, dryRun)
			}, printArchived)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm stopping the epics' live sessions")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "run every rule and change nothing; lists the sessions that would stop")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured results as JSON")
	return cmd
}

func printArchived(w io.Writer, r wire.TargetResult) error {
	verb := "archived"
	if r.Status == wire.TargetFits {
		verb = "would archive"
	}
	line := verb + " " + r.Target
	if r.NewAddress != "" {
		line += " as " + r.NewAddress
	}
	if r.OnTrunk != nil && !*r.OnTrunk {
		line += " (not seen on trunk)"
	}
	if len(r.Sessions) > 0 {
		line += "; sessions: " + strings.Join(r.Sessions, ", ")
	}
	_, err := fmt.Fprintln(w, line)
	return err
}

// newServerEpicsUnarchiveCmd brings archived epics and Designs back; the target is the archive address.
func newServerEpicsUnarchiveCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "unarchive <project:epic>...",
		Short: "bring archived epics and Designs back out of the project's .archive",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runTargets(c, d, jsonOut, args, func(ctx context.Context, cl *apiclient.Client) (wire.TargetsResult, error) {
				return cl.Admin.UnarchiveEpics(ctx, args)
			}, printUnarchived)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured results as JSON")
	return cmd
}

func printUnarchived(w io.Writer, r wire.TargetResult) error {
	line := "unarchived " + r.Target
	if r.NewAddress != "" {
		line += " as " + r.NewAddress
	}
	_, err := fmt.Fprintln(w, line)
	return err
}

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
