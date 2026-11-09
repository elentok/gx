package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/settingsapi"
	"github.com/spf13/cobra"
)

func newServerSettingsCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var project string
	c := &cobra.Command{
		Use:   "settings",
		Short: "Show the settings and their values, globally or for one project",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			target := settingsapi.GlobalTarget
			if project != "" {
				target = project
			}
			return runServerSettings(c.Context(), cl, c.OutOrStdout(), target, jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&project, "project", "", "show one project's settings (default: the global ones)")
	return c
}

func runServerSettings(ctx context.Context, cl *apiclient.Client, w io.Writer, target string, jsonOut bool) error {
	resp, err := cl.Admin.Settings(ctx, target)
	if err != nil {
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, resp, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	for _, e := range resp.Entries {
		switch {
		case !e.Set:
			fmt.Fprintf(w, "%s: not set\n", e.Key)
		case len(e.Value) == 0:
			fmt.Fprintf(w, "%s: set\n", e.Key)
		default:
			fmt.Fprintf(w, "%s: %s\n", e.Key, e.Value)
		}
	}
	return nil
}
