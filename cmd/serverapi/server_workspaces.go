package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

// newServerWorkspacesCmd lists the workspaces and holds the verbs that change them.
func newServerWorkspacesCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspaces",
		Short: "list the workspaces, or create, rename or delete one through the gx server",
		Args:  cobra.NoArgs,
	}
	var listJSON bool
	list := &cobra.Command{
		Use:   "list",
		Short: "list the workspaces in order",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), listJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.WorkspaceList, error) {
					return cl.Reads.Workspaces(ctx)
				}, printWorkspaces)
		},
	}
	list.Flags().BoolVar(&listJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(list)

	write := func(use, short string, args cobra.PositionalArgs, do func(context.Context, *apiclient.Client, []string) (wire.WorkspaceResult, error)) {
		var jsonOut bool
		v := &cobra.Command{
			Use:   use,
			Short: short,
			Args:  args,
			RunE: func(c *cobra.Command, args []string) error {
				ctx := c.Context()
				if ctx == nil {
					ctx = context.Background()
				}
				cl, err := d.Dial()()
				if err != nil {
					return err
				}
				return runWorkspaceWrite(ctx, c.OutOrStdout(), jsonOut, func() (wire.WorkspaceResult, error) { return do(ctx, cl, args) })
			},
		}
		v.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
		cmd.AddCommand(v)
	}
	write("create <name>", "add a workspace last", cobra.ExactArgs(1),
		func(ctx context.Context, cl *apiclient.Client, a []string) (wire.WorkspaceResult, error) {
			return cl.Admin.CreateWorkspace(ctx, a[0])
		})
	write("rename <name> <new-name>", "rename a workspace; its members follow", cobra.ExactArgs(2),
		func(ctx context.Context, cl *apiclient.Client, a []string) (wire.WorkspaceResult, error) {
			return cl.Admin.RenameWorkspace(ctx, a[0], a[1])
		})
	write("delete <name>", "delete a workspace; its members move to the Default workspace", cobra.ExactArgs(1),
		func(ctx context.Context, cl *apiclient.Client, a []string) (wire.WorkspaceResult, error) {
			return cl.Admin.DeleteWorkspace(ctx, a[0])
		})
	write("reorder <name>...", "set the workspace order; list every workspace once", cobra.MinimumNArgs(1),
		func(ctx context.Context, cl *apiclient.Client, a []string) (wire.WorkspaceResult, error) {
			return cl.Admin.ReorderWorkspaces(ctx, a)
		})
	write("move <key> <workspace>", "put a Design, epic (project:epic) or chat session in a workspace", cobra.ExactArgs(2),
		func(ctx context.Context, cl *apiclient.Client, a []string) (wire.WorkspaceResult, error) {
			return cl.Admin.MoveToWorkspace(ctx, a[0], a[1])
		})
	write("remove <key>", "send a Design, epic or chat back to the Default workspace", cobra.ExactArgs(1),
		func(ctx context.Context, cl *apiclient.Client, a []string) (wire.WorkspaceResult, error) {
			return cl.Admin.RemoveFromWorkspace(ctx, a[0])
		})
	return cmd
}

// runWorkspaceWrite prints the result of one workspace write. With --json the
// result or refusal is the output; otherwise a refusal is an error.
func runWorkspaceWrite(_ context.Context, w io.Writer, jsonOut bool, do func() (wire.WorkspaceResult, error)) error {
	res, err := do()
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.WorkspaceResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	return printWorkspaces(w, wire.WorkspaceList{Workspaces: res.Workspaces})
}

func printWorkspaces(w io.Writer, l wire.WorkspaceList) error {
	for _, ws := range l.Workspaces {
		def := ""
		if ws.Default {
			def = "\tdefault"
		}
		if _, err := fmt.Fprintf(w, "%s\t%d%s\n", ws.Name, ws.Members, def); err != nil {
			return err
		}
	}
	return nil
}
