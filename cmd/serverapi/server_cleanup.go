package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
)

func newServerCleanupCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var project string
	c := &cobra.Command{
		Use:   "cleanup",
		Short: "Show a project's cleanup scan with the worktree sizes that are ready",
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
			return runServerCleanup(ctx, cl, c.OutOrStdout(), project, jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&project, "project", "", "the project to scan")
	_ = c.MarkFlagRequired("project")
	return c
}

func runServerCleanup(ctx context.Context, cl *apiclient.Client, w io.Writer, project string, jsonOut bool) error {
	rep, err := cl.Reads.Cleanup(ctx, project)
	if err != nil {
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, rep, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	for _, wt := range rep.Worktrees {
		size := "size pending"
		if wt.SizeBytes != nil {
			size = config.ByteSize(*wt.SizeBytes).String()
		}
		name := wt.Branch
		if name == "" {
			name = wt.Head
		}
		if _, err := fmt.Fprintf(w, "%s (%s): %s\n", name, wt.Kind, size); err != nil {
			return err
		}
	}
	return nil
}

func newServerCleanupAgentCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var project string
	c := &cobra.Command{
		Use:   "cleanup-agent",
		Short: "Start the cleanup agent on the branches and epics no button can decide",
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
			res, err := cl.Reads.StartCleanupAgent(ctx, wire.CleanupWriteRequest{Project: project})
			switch {
			case apiclient.IsNotRunning(err):
				res = wire.OneOffResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
			case err != nil:
				return fmt.Errorf("server write failed: %w", err)
			}
			w := c.OutOrStdout()
			if jsonOut {
				if err := cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd)); err != nil {
					return err
				}
			} else if !res.Refused {
				if _, err := fmt.Fprintf(w, "started %s\n", res.Address); err != nil {
					return err
				}
			}
			if res.Refused {
				return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&project, "project", "", "the project")
	_ = c.MarkFlagRequired("project")
	return c
}
