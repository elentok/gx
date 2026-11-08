package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

// newServerTriageCmd groups the Triage verbs that talk to the server.
func newServerTriageCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "triage", Short: "Triage calls to the running server"}

	var jsonOut bool
	changed := &cobra.Command{
		Use:   "changed <project>",
		Short: "tell the server a project's Triage record changed so the web updates at once",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			if err := cl.Triage.Changed(ctx, args[0]); err != nil {
				return fmt.Errorf("server ping failed: %w", err)
			}
			if jsonOut {
				return cmdbase.WriteStamped(c.OutOrStdout(), wire.TriageChangedResult{Project: args[0]}, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
			}
			return nil
		},
	}
	changed.Flags().BoolVar(&jsonOut, "json", false, "emit the acknowledged project as JSON")
	cmd.AddCommand(changed)

	var showJSON bool
	show := &cobra.Command{
		Use:   "show <project>",
		Short: "show a project's Triage as the web page reads it",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), showJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.TriageView, error) {
					return cl.Triage.Show(ctx, args[0])
				},
				func(w io.Writer, v wire.TriageView) error {
					_, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d rows\t%s\n", v.Project, v.State, v.StepNote, len(v.Rows), v.ID)
					return err
				})
		},
	}
	show.Flags().BoolVar(&showJSON, "json", false, "emit the whole view as JSON")
	cmd.AddCommand(show)
	return cmd
}
