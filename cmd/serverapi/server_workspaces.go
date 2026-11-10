package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

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
				return runWorkspaceWrite(ctx, c.OutOrStdout(), jsonOut, func(w io.Writer, l []wire.Workspace) error {
					return printWorkspaces(w, wire.WorkspaceList{Workspaces: l})
				}, func() (wire.WorkspaceResult, error) { return do(ctx, cl, args) })
			},
		}
		v.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
		cmd.AddCommand(v)
	}
	cmd.AddCommand(newWorkspaceSortCmd(d))
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
	cmd.AddCommand(newWorkspaceMoveCmd(d))
	write("remove <key>", "send a Design, epic or chat back to the Default workspace", cobra.ExactArgs(1),
		func(ctx context.Context, cl *apiclient.Client, a []string) (wire.WorkspaceResult, error) {
			return cl.Admin.RemoveFromWorkspace(ctx, a[0])
		})
	return cmd
}

// newWorkspaceMoveCmd puts many Designs, epics or chats in one workspace, each on its own.
func newWorkspaceMoveCmd(d Deps) *cobra.Command {
	var jsonOut bool
	v := &cobra.Command{
		Use:   "move <key>... <workspace>",
		Short: "put Designs, epics (project:epic) or chat sessions in a workspace (the workspace is last)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			keys, workspace := args[:len(args)-1], args[len(args)-1]
			res, err := cl.Admin.MoveToWorkspace(ctx, workspace, keys...)
			switch {
			case apiclient.IsNotRunning(err):
				res = wire.TargetsResult{}
				for _, k := range keys {
					res.Results = append(res.Results, notRunning(k))
				}
			case err != nil:
				return fmt.Errorf("server write failed: %w", err)
			}
			return cmdbase.PrintTargets(c.OutOrStdout(), jsonOut, res, res.Results, func(w io.Writer, r wire.TargetResult) error {
				_, err := fmt.Fprintf(w, "done %s\n", r.Target)
				return err
			})
		},
	}
	v.Flags().BoolVar(&jsonOut, "json", false, "emit the structured results as JSON")
	return v
}

// newWorkspaceSortCmd reads a workspace's Sort, or sets it when a value is given.
func newWorkspaceSortCmd(d Deps) *cobra.Command {
	var jsonOut bool
	v := &cobra.Command{
		Use:   "sort <workspace> [value]",
		Short: "show a workspace's Sort, or set it to needs-you, activity or name",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			name := args[0]
			show := func(w io.Writer, l []wire.Workspace) error {
				for _, ws := range l {
					if strings.EqualFold(ws.Name, name) {
						_, err := fmt.Fprintln(w, sortOrDefault(ws.Sort))
						return err
					}
				}
				return fmt.Errorf("refused (%s): no workspace named %q", wire.ReasonWorkspaceUnknown, name)
			}
			if len(args) == 2 {
				return runWorkspaceWrite(ctx, c.OutOrStdout(), jsonOut, show, func() (wire.WorkspaceResult, error) {
					return cl.Admin.SetWorkspaceSort(ctx, name, args[1])
				})
			}
			l, err := cl.Reads.Workspaces(ctx)
			if apiclient.IsNotRunning(err) {
				return fmt.Errorf("no server is running; start it with `gx server start`")
			}
			if err != nil {
				return fmt.Errorf("server read failed: %w", err)
			}
			return show(c.OutOrStdout(), l.Workspaces)
		},
	}
	v.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON when setting")
	return v
}

func sortOrDefault(s string) string {
	if s == "" {
		return wire.SortDefault
	}
	return s
}

// runWorkspaceWrite prints the result of one workspace write through show. With --json the
// result or refusal is the output; otherwise a refusal is an error.
func runWorkspaceWrite(_ context.Context, w io.Writer, jsonOut bool, show func(io.Writer, []wire.Workspace) error, do func() (wire.WorkspaceResult, error)) error {
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
	return show(w, res.Workspaces)
}

func printWorkspaces(w io.Writer, l wire.WorkspaceList) error {
	for _, ws := range l.Workspaces {
		def := ""
		if ws.Default {
			def = "\tdefault"
		}
		if _, err := fmt.Fprintf(w, "%s\t%d\t%d needs you%s\n", ws.Name, ws.Members, ws.NeedsYou, def); err != nil {
			return err
		}
	}
	return nil
}
