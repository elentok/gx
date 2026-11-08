package serverapi

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

type cleanupWrite func(*apiclient.Reads, context.Context, wire.CleanupWriteRequest) (wire.CleanupWriteResult, error)

func newServerCleanupRemoveCmd(d Deps) *cobra.Command {
	return newServerCleanupWriteCmd(d, "cleanup-remove", "Remove a merged branch's worktree, then the branch", (*apiclient.Reads).RemoveCleanupWorktree)
}

func newServerCleanupRemoveUnmergedCmd(d Deps) *cobra.Command {
	return newServerCleanupWriteCmd(d, "cleanup-remove-unmerged", "Remove an unmerged branch's clean, idle worktree and keep the branch", (*apiclient.Reads).RemoveUnmergedCleanupWorktree)
}

func newServerCleanupArchiveCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var project, epic string
	c := &cobra.Command{
		Use:   "cleanup-archive",
		Short: "Archive a done epic that has landed on trunk",
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
			return runServerCleanupWrite(ctx, cl, c.OutOrStdout(), (*apiclient.Reads).ArchiveCleanupEpic, wire.CleanupWriteRequest{Project: project, Epic: epic}, jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&project, "project", "", "the project")
	c.Flags().StringVar(&epic, "epic", "", "the epic")
	_ = c.MarkFlagRequired("project")
	_ = c.MarkFlagRequired("epic")
	return c
}

func newServerCleanupDeleteCmd(d Deps) *cobra.Command {
	return newServerCleanupWriteCmd(d, "cleanup-delete", "Delete a merged branch that has no worktree", (*apiclient.Reads).DeleteCleanupBranch)
}

func newServerCleanupWriteCmd(d Deps, use, short string, call cleanupWrite) *cobra.Command {
	var jsonOut bool
	var project, branch string
	c := &cobra.Command{
		Use:   use,
		Short: short,
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
			return runServerCleanupWrite(ctx, cl, c.OutOrStdout(), call, wire.CleanupWriteRequest{Project: project, Branch: branch}, jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&project, "project", "", "the project")
	c.Flags().StringVar(&branch, "branch", "", "the branch")
	_ = c.MarkFlagRequired("project")
	_ = c.MarkFlagRequired("branch")
	return c
}

// runServerCleanupWrite prints the result. A refusal returns an error, so the
// command exits non-zero.
func runServerCleanupWrite(ctx context.Context, cl *apiclient.Client, w io.Writer, call cleanupWrite, req wire.CleanupWriteRequest, jsonOut bool) error {
	res, err := call(cl.Reads, ctx, req)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.CleanupWriteResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		if err := cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd)); err != nil {
			return err
		}
	} else if !res.Refused {
		if _, err := fmt.Fprintf(w, "cleaned %s\n", cmp.Or(req.Branch, req.Epic)); err != nil {
			return err
		}
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	return nil
}
