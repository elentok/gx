package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

func newServerConfigStatusCmd(d deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "config-status",
		Short: "Show the last config reload, startup-only changes waiting for a restart and any hard-limit countdown",
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
			return runConfigStatus(ctx, cl, c.OutOrStdout(), jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	return c
}

func runConfigStatus(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool) error {
	st, err := cl.ConfigStatus(ctx)
	if err != nil {
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, st, viaServer, callerActor(os.Getwd))
	}
	if st.Empty() {
		_, err = fmt.Fprintln(w, "no reload since startup")
		return err
	}
	return writeConfigBlock(w, st, time.Now())
}

// writeConfigBlock renders the Config block that both `gx server
// config-status` and `gx server status` print.
func writeConfigBlock(w io.Writer, st server.ConfigStatus, now time.Time) error {
	lines := []string{"config:"}
	if r := st.LastReload; r != nil {
		lines = append(lines, fmt.Sprintf("last reload: %s (%s, %s ago): %s", r.Outcome, r.Trigger, now.Sub(r.At).Round(time.Second), r.Message))
	}
	if len(st.PendingRestart) > 0 {
		lines = append(lines, "restart needed for: "+strings.Join(st.PendingRestart, ", "))
	}
	if !st.HardStopAt.IsZero() {
		lines = append(lines, fmt.Sprintf("hard limit: agents stop in %s unless it is raised", max(st.HardStopAt.Sub(now), 0).Round(time.Second)))
	}
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n  "))
	return err
}
