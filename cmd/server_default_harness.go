package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// followGlobal is the CLI spelling of removing a project's override.
const followGlobal = "follow-global"

func newServerDefaultHarnessCmd(d deps) *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "default-harness",
		Short: "Show the Default harness, the agent runner and each project's override",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			return runServerDefaultHarness(cmdContext(c), cl, c.OutOrStdout(), jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	return c
}

func cmdContext(c *cobra.Command) context.Context {
	if ctx := c.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func runServerDefaultHarness(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool) error {
	st, err := cl.DefaultHarness(ctx)
	if err != nil {
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, st, viaServer, callerActor(os.Getwd))
	}
	fmt.Fprintf(w, "default: %s (agent-runner: %s)\n", st.Default, st.Runner)
	for _, p := range st.Projects {
		if p.Harness != "" {
			fmt.Fprintf(w, "%s: %s\n", p.Project, p.Harness)
		}
	}
	return nil
}

func newServerDefaultHarnessSetCmd(d deps) *cobra.Command {
	var jsonOut bool
	var project string
	c := &cobra.Command{
		Use:   "default-harness-set <claude|codex|" + followGlobal + ">",
		Short: "Set the Default harness, globally or for one project",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			req := server.SetDefaultHarnessRequest{Scope: server.DefaultHarnessGlobal, Harness: args[0]}
			if project != "" {
				req.Scope, req.Project = server.DefaultHarnessProject, project
			}
			if args[0] == followGlobal {
				if project == "" {
					return fmt.Errorf("%s needs --project", followGlobal)
				}
				req.Harness = ""
			}
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			return runServerDefaultHarnessSet(cmdContext(c), cl, c.OutOrStdout(), req, jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&project, "project", "", "set the override of one project (default: the global default)")
	return c
}

func runServerDefaultHarnessSet(ctx context.Context, cl *apiclient.Client, w io.Writer, req server.SetDefaultHarnessRequest, jsonOut bool) error {
	res, err := cl.SetDefaultHarness(ctx, req)
	if err != nil {
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, res, viaServer, callerActor(os.Getwd))
	}
	switch {
	case res.Refused:
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	case res.Notice != "":
		fmt.Fprintln(w, res.Notice)
	default:
		fmt.Fprintf(w, "default is already %s\n", res.Harness)
	}
	return nil
}
