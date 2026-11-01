package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
)

func newAgentsCommandsCmd(d deps) *cobra.Command {
	var jsonOut bool
	var kind, project, dir string
	cmd := &cobra.Command{
		Use:   "commands [<session>] [--kind '$'] | --project <p> --dir <d>",
		Short: "list the slash commands (or $ skills) a session or project dir offers",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			if (id == "") == (project == "") {
				return fmt.Errorf("give either a session or --project with --dir")
			}
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			return runAgentsCommands(c.Context(), cl, c.OutOrStdout(), jsonOut, id, project, dir, kind)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.Flags().StringVar(&kind, "kind", "/", "which list: / for commands, $ for skills")
	cmd.Flags().StringVar(&project, "project", "", "list for a First prompt in this project (with --dir)")
	cmd.Flags().StringVar(&dir, "dir", "", "the project root or one of its worktrees")
	return cmd
}

func runAgentsCommands(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, id, project, dir, kind string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.SessionCommands(ctx, id, project, dir, kind)
	if err != nil {
		return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, res, viaServer, callerActor(os.Getwd))
	}
	for _, e := range res.Entries {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Name, e.ArgHint, e.Source, e.Description); err != nil {
			return err
		}
	}
	if res.Degraded {
		_, err := fmt.Fprintln(w, "(degraded: the harness did not answer, built-ins only)")
		return err
	}
	return nil
}
