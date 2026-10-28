package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

func newServerReloadConfigCmd(d deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "reload-config",
		Short: "Reload the global config.json into the running server",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runReloadConfig(ctx, cl, c.OutOrStdout(), jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return c
}

func runReloadConfig(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool) error {
	res, err := cl.ReloadConfig(ctx)
	switch {
	case apiclient.IsNotRunning(err):
		res = server.ReloadResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "server not running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, res, viaServer, callerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	fmt.Fprintln(w, "gx config reloaded successfully")
	if len(res.Changed) == 0 {
		fmt.Fprintln(w, "no changes")
	} else {
		fmt.Fprintln(w, "changed: "+strings.Join(res.Changed, ", "))
	}
	for _, warning := range res.Warnings {
		fmt.Fprintln(w, "warning: "+warning)
	}
	return nil
}
