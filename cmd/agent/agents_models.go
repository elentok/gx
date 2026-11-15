package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
)

func newAgentsModelsCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "models <session>",
		Short: "list the models a session can switch to",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runAgentsModels(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func runAgentsModels(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, id string) error {
	res, err := cl.Sessions.Models(ctxOrBackground(ctx), id)
	if err != nil {
		return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	for _, m := range res.Models {
		mark := ""
		if m.Current {
			mark = "*"
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.ID, mark, strings.Join(m.Efforts, ","), m.Latest); err != nil {
			return err
		}
	}
	if res.Refreshing {
		_, err := fmt.Fprintln(w, "(refreshing: ask again in a moment)")
		return err
	}
	return nil
}
