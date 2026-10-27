package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/spf13/cobra"
)

func newServerChecksCmd() *cobra.Command {
	var jsonOut bool
	var project string
	c := &cobra.Command{
		Use:   "checks",
		Short: "Show each project's check config and the check commands gx would recommend",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runServerChecks(ctx, cl, c.OutOrStdout(), project, jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&project, "project", "", "show one project (default: every registered project)")
	return c
}

func runServerChecks(ctx context.Context, cl *apiclient.Client, w io.Writer, project string, jsonOut bool) error {
	st, err := cl.ChecksStatus(ctx, project)
	if err != nil {
		return fmt.Errorf("server read failed: %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, st, viaServer, callerActor(os.Getwd))
	}
	for _, p := range st.Projects {
		state := "configured"
		switch {
		case p.Error != "":
			state = "broken: " + p.Error
		case p.Missing:
			state = "missing"
		}
		fmt.Fprintf(w, "%s: %s\n", p.Project, state)
		for _, k := range p.Kinds {
			line := fmt.Sprintf("  %s: %s", k.Kind, k.State)
			if k.Recommended != nil {
				line += fmt.Sprintf(", recommended %s (%s)", k.Recommended.Command, k.Recommended.Source)
			} else {
				line += ", gx default runs " + k.Default
			}
			fmt.Fprintln(w, line)
		}
	}
	return nil
}
