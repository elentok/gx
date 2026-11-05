package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

func newServerRestartNowCmd(d Deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "restart-now",
		Short: "Restart the server from the restart binary, as the web UI's restart button does",
		Long: "Runs the restart check, then restarts with the values it shows. The server refuses when\n" +
			"the check changed in between, or while another restart runs.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runServerRestartNow(ctx, cl, c.OutOrStdout(), jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	return c
}

func runServerRestartNow(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool) error {
	rep, err := cl.Admin.RestartCheck(ctx)
	if err != nil {
		return fmt.Errorf("server read failed: %w", err)
	}
	if rep.Status != wire.RestartCheckOK {
		return fmt.Errorf("restart check failed (%s): %s", rep.Status, rep.Error)
	}
	res, err := cl.Admin.Restart(ctx, restartRequestFor(rep))
	if err != nil {
		return fmt.Errorf("server restart failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintf(w, "restarting; log: %s\n", res.RestartLog)
	return err
}

// restartRequestFor confirms the values a check showed.
func restartRequestFor(rep wire.RestartCheckReport) wire.RestartRequest {
	req := wire.RestartRequest{NewVersion: rep.NewVersion, ConfigOK: rep.Config.OK, FailingProjects: []string{}}
	for _, p := range rep.Projects {
		if !p.OK {
			req.FailingProjects = append(req.FailingProjects, p.Name)
		}
	}
	return req
}
