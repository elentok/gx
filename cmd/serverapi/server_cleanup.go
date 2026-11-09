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
