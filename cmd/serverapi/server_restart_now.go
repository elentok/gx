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
		if err := cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd)); err != nil {
			return err
		}
		return cmdbase.JSONRefusalExit(res.Refused)
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

func newServerRestartCheckCmd(d Deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "restart-check",
		Short: "Check what a restart would do: versions, config, migration dry run and running agents (the server keeps running)",
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
			rep, err := cl.Admin.RestartCheck(ctx)
			if err != nil {
				return fmt.Errorf("server read failed: %w", err)
			}
			if jsonOut {
				return cmdbase.EncodeProvenance(c.OutOrStdout(), rep, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
			}
			return writeRestartCheck(c.OutOrStdout(), rep)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	return c
}

func writeRestartCheck(w io.Writer, rep wire.RestartCheckReport) error {
	fmt.Fprintf(w, "running: %s\nbinary:  %s\n", rep.RunningVersion, rep.Binary)
	switch rep.Status {
	case wire.RestartCheckTooOld:
		fmt.Fprintf(w, "too old: %s\n%s\n", rep.Error, rep.Hint)
	case wire.RestartCheckUnreadable:
		fmt.Fprintf(w, "unreadable version: %s\n", rep.Error)
	default:
		fmt.Fprintf(w, "new:     %s\n", rep.NewVersion)
		if rep.Config.OK {
			fmt.Fprintln(w, "config:  ok")
		} else {
			fmt.Fprintf(w, "config:  failed: %s\n", rep.Config.Error)
		}
		for _, p := range rep.Projects {
			if p.OK {
				fmt.Fprintf(w, "project %s: ok (%d file changes)\n", p.Name, len(p.Files))
			} else {
				fmt.Fprintf(w, "project %s: failed: %s\n", p.Name, p.Error)
			}
		}
	}
	_, err := fmt.Fprintf(w, "agents running: %d\ninstance: %s\nrestart log: %s\n", rep.Agents, rep.Instance, rep.RestartLog)
	return err
}
