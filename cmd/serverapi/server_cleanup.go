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
	"github.com/elentok/gx/server/wire/ops"
)

type cleanupWrite func(*apiclient.Reads, context.Context, ops.CleanupWriteRequest) (ops.CleanupWriteResult, error)

// extraFlags registers a command's own flags on req and returns the name of the
// one that is required.
type extraFlags func(c *cobra.Command, req *ops.CleanupWriteRequest) (required string)

func branchFlag(c *cobra.Command, req *ops.CleanupWriteRequest) string {
	c.Flags().StringVar(&req.Branch, "branch", "", "the branch")
	return "branch"
}

func epicFlags(c *cobra.Command, req *ops.CleanupWriteRequest) string {
	c.Flags().StringVar(&req.Epic, "epic", "", "the epic")
	c.Flags().BoolVar(&req.Yes, "yes", false, "confirm stopping the epic's live sessions")
	return "epic"
}

// newCleanupCmd builds a cleanup command with --json and the required --project
// plus, when extra is not nil, its flags and its required flag.
func newCleanupCmd(d Deps, use, short string, extra extraFlags, run func(ctx context.Context, cl *apiclient.Client, w io.Writer, req ops.CleanupWriteRequest, jsonOut bool) error) *cobra.Command {
	var jsonOut bool
	var req ops.CleanupWriteRequest
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
	mustRequire(c, "project")
	if extra != nil {
		mustRequire(c, extra(c, &req))
	}
	return c
}

// mustRequire panics when name is not a registered flag: that is a wiring bug.
func mustRequire(c *cobra.Command, name string) {
	if err := c.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}

func newServerCleanupRemoveCmd(d Deps) *cobra.Command {
	return newServerCleanupWriteCmd(d, "cleanup-remove", "Remove a merged branch's worktree, then the branch", branchFlag, (*apiclient.Reads).RemoveCleanupWorktree)
}

func newServerCleanupRemoveUnmergedCmd(d Deps) *cobra.Command {
	return newServerCleanupWriteCmd(d, "cleanup-remove-unmerged", "Remove an unmerged branch's clean, idle worktree and keep the branch", branchFlag, (*apiclient.Reads).RemoveUnmergedCleanupWorktree)
}

func newServerCleanupArchiveCmd(d Deps) *cobra.Command {
	return newServerCleanupWriteCmd(d, "cleanup-archive", "Archive a done epic (one-epic alias of `gx server epics archive`)", epicFlags, (*apiclient.Reads).ArchiveCleanupEpic)
}

func newServerCleanupDeleteCmd(d Deps) *cobra.Command {
	return newServerCleanupWriteCmd(d, "cleanup-delete", "Delete a merged branch that has no worktree", branchFlag, (*apiclient.Reads).DeleteCleanupBranch)
}

func newServerCleanupWriteCmd(d Deps, use, short string, extra extraFlags, call cleanupWrite) *cobra.Command {
	return newCleanupCmd(d, use, short, extra, func(ctx context.Context, cl *apiclient.Client, w io.Writer, req ops.CleanupWriteRequest, jsonOut bool) error {
		return runServerCleanupWrite(ctx, cl, w, call, req, jsonOut)
	})
}

// runServerCleanupWrite prints the result. A refusal returns an error, so the
// command exits non-zero.
func runServerCleanupWrite(ctx context.Context, cl *apiclient.Client, w io.Writer, call cleanupWrite, req ops.CleanupWriteRequest, jsonOut bool) error {
	return runWebCall(ctx, func(ctx context.Context) (ops.CleanupWriteResult, error) { return call(cl.Reads, ctx, req) }, "server write failed", jsonOut, w, func(ops.CleanupWriteResult) error {
		_, err := fmt.Fprintf(w, "cleaned %s\n", cmp.Or(req.Branch, req.Epic))
		return err
	})
}

func newServerCleanupCmd(d Deps) *cobra.Command {
	return newCleanupCmd(d, "cleanup", "Show a project's cleanup scan with the worktree sizes that are ready", nil,
		func(ctx context.Context, cl *apiclient.Client, w io.Writer, req ops.CleanupWriteRequest, jsonOut bool) error {
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
	return newCleanupCmd(d, "cleanup-agent", "Start the cleanup agent on the branches and epics no button can decide", nil,
		func(ctx context.Context, cl *apiclient.Client, w io.Writer, req ops.CleanupWriteRequest, jsonOut bool) error {
			return runWebCall(ctx, func(ctx context.Context) (wire.OneOffResult, error) {
				return cl.Reads.StartCleanupAgent(ctx, req)
			}, "server write failed", jsonOut, w, func(res wire.OneOffResult) error {
				_, err := fmt.Fprintf(w, "started %s\n", res.Address)
				return err
			})
		})
}
