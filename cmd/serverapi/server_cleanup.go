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
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
)

type cleanupWrite func(*apiclient.Reads, context.Context, wire.CleanupWriteRequest) (wire.CleanupWriteResult, error)

// newCleanupCmd builds a cleanup command with --json and the required --project
// plus, when extra is "branch" or "epic", that required flag too.
func newCleanupCmd(d Deps, use, short, extra string, run func(ctx context.Context, cl *apiclient.Client, w io.Writer, req wire.CleanupWriteRequest, jsonOut bool) error) *cobra.Command {
	var jsonOut bool
	var req wire.CleanupWriteRequest
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, ctx, err := webClient(d, c)
			if err != nil {
				return err
			}
			return run(ctx, cl, c.OutOrStdout(), req, jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&req.Project, "project", "", "the project")
	_ = c.MarkFlagRequired("project")
	switch extra {
	case "branch":
		c.Flags().StringVar(&req.Branch, "branch", "", "the branch")
	case "epic":
		c.Flags().StringVar(&req.Epic, "epic", "", "the epic")
		c.Flags().BoolVar(&req.Yes, "yes", false, "confirm stopping the epic's live sessions")
	}
	if extra != "" {
		_ = c.MarkFlagRequired(extra)
	}
	return c
}

func newServerCleanupRemoveCmd(d Deps) *cobra.Command {
	return newServerCleanupWriteCmd(d, "cleanup-remove", "Remove a merged branch's worktree, then the branch", "branch", (*apiclient.Reads).RemoveCleanupWorktree)
}

func newServerCleanupRemoveUnmergedCmd(d Deps) *cobra.Command {
	return newServerCleanupWriteCmd(d, "cleanup-remove-unmerged", "Remove an unmerged branch's clean, idle worktree and keep the branch", "branch", (*apiclient.Reads).RemoveUnmergedCleanupWorktree)
}

func newServerCleanupArchiveCmd(d Deps) *cobra.Command {
	return newServerCleanupWriteCmd(d, "cleanup-archive", "Archive a done epic (one-epic alias of `gx server epics archive`)", "epic", (*apiclient.Reads).ArchiveCleanupEpic)
}

func newServerCleanupDeleteCmd(d Deps) *cobra.Command {
	return newServerCleanupWriteCmd(d, "cleanup-delete", "Delete a merged branch that has no worktree", "branch", (*apiclient.Reads).DeleteCleanupBranch)
}

func newServerCleanupWriteCmd(d Deps, use, short, extra string, call cleanupWrite) *cobra.Command {
	return newCleanupCmd(d, use, short, extra, func(ctx context.Context, cl *apiclient.Client, w io.Writer, req wire.CleanupWriteRequest, jsonOut bool) error {
		return runServerCleanupWrite(ctx, cl, w, call, req, jsonOut)
	})
}

// runServerCleanupWrite prints the result. A refusal returns an error, so the
// command exits non-zero.
func runServerCleanupWrite(ctx context.Context, cl *apiclient.Client, w io.Writer, call cleanupWrite, req wire.CleanupWriteRequest, jsonOut bool) error {
	return runWebCall(ctx, func(ctx context.Context) (wire.CleanupWriteResult, error) { return call(cl.Reads, ctx, req) }, "server write failed", jsonOut, w, func(wire.CleanupWriteResult) error {
		_, err := fmt.Fprintf(w, "cleaned %s\n", cmp.Or(req.Branch, req.Epic))
		return err
	})
}

func newServerCleanupCmd(d Deps) *cobra.Command {
	return newCleanupCmd(d, "cleanup", "Show a project's cleanup scan with the worktree sizes that are ready", "",
		func(ctx context.Context, cl *apiclient.Client, w io.Writer, req wire.CleanupWriteRequest, jsonOut bool) error {
			return runServerCleanup(ctx, cl, w, req.Project, jsonOut)
		})
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
	return newCleanupCmd(d, "cleanup-agent", "Start the cleanup agent on the branches and epics no button can decide", "",
		func(ctx context.Context, cl *apiclient.Client, w io.Writer, req wire.CleanupWriteRequest, jsonOut bool) error {
			return runWebCall(ctx, func(ctx context.Context) (wire.OneOffResult, error) {
				return cl.Reads.StartCleanupAgent(ctx, req)
			}, "server write failed", jsonOut, w, func(res wire.OneOffResult) error {
				_, err := fmt.Fprintf(w, "started %s\n", res.Address)
				return err
			})
		})
}
