package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
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
