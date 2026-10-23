package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// newDesignsCmd is `gx designs`: the Designs, the sessions that run gx-design
// on a map epic.
func newDesignsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "designs", Short: "map epics' design sessions owned by the server", Args: cobra.NoArgs}
	cmd.AddCommand(newDesignsOpenCmd())
	return cmd
}

// runDesignsOpen never starts the server: a dead socket is a refusal.
func runDesignsOpen(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req server.DesignOpenRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.OpenDesign(ctx, req)
	return printSessionResult(w, jsonOut, res, err)
}

// designOpenRequest builds the request for `gx designs open`. --idea and
// --project start a New Design, where the argument is the slug the map will
// take; they go together and exclude --new.
func designOpenRequest(arg, idea, project string, fresh bool) (server.DesignOpenRequest, error) {
	req := server.DesignOpenRequest{Epic: arg, New: fresh}
	if idea == "" && project == "" {
		return req, nil
	}
	switch {
	case idea == "":
		return req, fmt.Errorf("--project needs --idea")
	case project == "":
		return req, fmt.Errorf("--idea needs --project")
	case fresh:
		return req, fmt.Errorf("--new cannot be combined with --idea and --project")
	}
	req.NewDesign, req.Idea, req.Project = true, idea, project
	return req, nil
}

func newDesignsOpenCmd() *cobra.Command {
	var jsonOut, fresh bool
	var idea, project string
	cmd := &cobra.Command{
		Use:   "open <project:epic[/NN]> [--new] | open <slug> --idea <text> --project <project>",
		Short: "print the Design's live session, starting one if there is none",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			req, err := designOpenRequest(args[0], idea, project, fresh)
			if err != nil {
				return err
			}
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runDesignsOpen(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().BoolVar(&fresh, "new", false, "stop the live session and start a fresh one")
	cmd.Flags().StringVar(&idea, "idea", "", "start a New Design for this idea (the argument is the map's slug)")
	cmd.Flags().StringVar(&project, "project", "", "project the New Design's map will live in")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}
