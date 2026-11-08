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

// newServerTipsCmd shows the tips the web UI's Tips page lists.
func newServerTipsCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "tips",
		Short: "show the tips for the server machine's OS",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.Tip, error) { return cl.Reads.Tips(ctx) },
				printTips)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func printTips(w io.Writer, tips []wire.Tip) error {
	for _, t := range tips {
		if _, err := fmt.Fprintf(w, "%s\n%s\n", t.Title, t.Body); err != nil {
			return err
		}
		for _, f := range t.Folders {
			if _, err := fmt.Fprintf(w, "  %s\n", f); err != nil {
				return err
			}
		}
	}
	return nil
}
