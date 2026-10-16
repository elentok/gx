package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

func newAFKCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:       "afk [on|off]",
		Short:     "toggle or set AFK mode: session banners also go to chat (asks the server)",
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"on", "off"},
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runAFK(ctx, cl, c.OutOrStdout(), jsonOut, args)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// runAFK mirrors runBudgetWrite; no args toggles.
func runAFK(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, args []string) error {
	var on *bool
	if len(args) == 1 {
		v := args[0] == "on"
		on = &v
	}
	res, err := cl.SetAFK(ctx, on)
	switch {
	case apiclient.IsNotRunning(err):
		res = server.AFKResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, res, viaServer, callerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	state := "off"
	if res.AFK {
		state = "on"
	}
	_, err = fmt.Fprintf(w, "afk %s\n", state)
	return err
}
