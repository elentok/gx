package ticketsflow

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/tickets/ticketspec"
)

func newTicketsDraftCmd(d Deps) *cobra.Command {
	var planPath, manifestPath string
	c := &cobra.Command{
		Use:   "draft <project:epic> --plan <plan.yaml> --manifest <manifest.yaml>",
		Short: "write a publish manifest and body skeletons from a draft plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if planPath == "" || manifestPath == "" {
				return fmt.Errorf("draft needs --plan <plan.yaml> and --manifest <manifest.yaml>")
			}
			return runDraft(planPath, manifestPath, c.OutOrStdout())
		},
	}
	c.Flags().StringVar(&planPath, "plan", "", "the draft plan (YAML)")
	c.Flags().StringVar(&manifestPath, "manifest", "", "publish manifest to write or extend; body files go next to it")
	return c
}

// runDraft renders every ticket of the plan. A re-run keeps the agent's parts
// of an existing body, and deletes the body file of a slug the plan dropped.
func runDraft(planPath, manifestPath string, w io.Writer) error {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	plan, err := ticketspec.ParseDraftPlan(data)
	if err != nil {
		return err
	}
	dir := filepath.Dir(manifestPath)
	rows := make([]ticketspec.ManifestRow, 0, len(plan.Tickets))
	bodies := map[string]string{}
	for _, t := range plan.Tickets {
		var what, accept string
		if old, err := os.ReadFile(filepath.Join(dir, t.BodyFileName())); err == nil {
			what, accept = ticketspec.AgentParts(string(old))
		}
		bodies[t.BodyFileName()] = ticketspec.RenderDraftBody(t, plan.Scenario, what, accept)
		rows = append(rows, ticketspec.ManifestRow{Slug: t.Slug, Type: t.Type, BlockedBy: t.BlockedBy, Window: t.Window, Body: t.BodyFileName()})
	}

	existing, err := os.ReadFile(manifestPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	out, removed, err := ticketspec.AppendDraftManifest(existing, rows)
	if err != nil {
		return err
	}
	for name, body := range bodies {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(manifestPath, out, 0o644); err != nil {
		return err
	}
	for _, name := range removed {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%s: %s\n", r.Slug, r.Body)
	}
	return nil
}
