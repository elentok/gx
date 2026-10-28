package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

func newServerChecksSaveCmd(d deps) *cobra.Command {
	var jsonOut bool
	var req server.SaveChecksRequest
	var checks string
	c := &cobra.Command{
		Use:   "checks-save",
		Short: "Save a project's check config to the repo or to project.json",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			req.Checks = json.RawMessage(checks)
			return runServerChecksSave(ctx, cl, c.OutOrStdout(), req, jsonOut)
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&req.Project, "project", "", "project to save for")
	c.Flags().StringVar(&req.Target, "target", server.SaveTargetRepo, "repo or project.json")
	c.Flags().StringVar(&checks, "checks", "", "the check config, as the JSON .gx/checks.json holds")
	_ = c.MarkFlagRequired("project")
	_ = c.MarkFlagRequired("checks")
	return c
}

func runServerChecksSave(ctx context.Context, cl *apiclient.Client, w io.Writer, req server.SaveChecksRequest, jsonOut bool) error {
	res, err := cl.SaveChecks(ctx, req)
	if err != nil {
		return fmt.Errorf("server save failed: %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, res, viaServer, callerActor(os.Getwd))
	}
	switch {
	case res.Refused:
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	case res.Commit != "":
		fmt.Fprintf(w, "saved to %s: commit %s on %s\n", res.Target, res.Commit, res.Trunk)
	case res.Path != "":
		fmt.Fprintf(w, "saved to %s\n", res.Path)
	default:
		fmt.Fprintf(w, "%s already holds this config\n", res.Target)
	}
	return nil
}

func newServerChecksCmd(d deps) *cobra.Command {
	var jsonOut bool
	var project string
	c := &cobra.Command{
		Use:   "checks",
		Short: "Show each project's check config and the check commands gx would recommend",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.dial()()
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
