package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

func newServerConfigStatusCmd(d Deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "config-status",
		Short: "Show the last config reload, startup-only changes waiting for a restart and any hard-limit countdown",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runConfigStatus(ctx, cl, c.OutOrStdout(), jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	return c
}

func runConfigStatus(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool) error {
	st, err := cl.Admin.ConfigStatus(ctx)
	if err != nil {
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, st, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if st.Empty() {
		_, err = fmt.Fprintln(w, "no reload since startup")
		return err
	}
	return cmdbase.WriteConfigBlock(w, st, time.Now())
}

func newServerReloadConfigCmd(d Deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "reload-config",
		Short: "Reload the global config.json into the running server",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.Dial()()
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
	res, err := cl.Admin.ReloadConfig(ctx)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.ReloadResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "server not running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		if err := cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd)); err != nil {
			return err
		}
		return cmdbase.JSONRefusalExit(res.Refused)
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
