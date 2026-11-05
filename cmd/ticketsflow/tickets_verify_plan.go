package ticketsflow

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/tickets/ticketspec"
)

func newTicketsVerifyPlanCmd(d Deps) *cobra.Command {
	var planPath, manifestPath string
	var table bool
	c := &cobra.Command{
		Use:   "verify-plan <project:epic> --plan <plan.yaml> [--manifest <manifest.yaml>] | --table",
		Short: "pack a verify plan into verify tickets, or print the estimate table",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if table {
				ticketspec.WriteEstimateTable(c.OutOrStdout())
				return nil
			}
			if len(args) != 1 || planPath == "" {
				return fmt.Errorf("verify-plan needs <project:epic> and --plan <plan.yaml>, or --table")
			}
			cwd, err := d.Getwd()
			if err != nil {
				return err
			}
			return runVerifyPlan(cwd, args[0], planPath, manifestPath, c.OutOrStdout())
		},
	}
	c.Flags().StringVar(&planPath, "plan", "", "the verify plan (YAML)")
	c.Flags().StringVar(&manifestPath, "manifest", "", "publish manifest to append the verify entries to (created when missing); body files go next to it")
	c.Flags().BoolVar(&table, "table", false, "print the estimate table and exit")
	return c
}

// specPathFor is the spec of epic <name>-impl: docs/specs/<name>.md.
func specPathFor(epicArg string) (string, error) {
	name := epicArg[strings.LastIndex(epicArg, ":")+1:]
	base, ok := strings.CutSuffix(name, "-impl")
	if !ok || base == "" {
		return "", fmt.Errorf("epic %q does not end in -impl, so it has no spec at docs/specs/<name>.md", name)
	}
	return filepath.Join("docs", "specs", base+".md"), nil
}

func runVerifyPlan(cwd, epicArg, planPath, manifestPath string, w io.Writer) error {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	plan, err := ticketspec.ParseVerifyPlan(data)
	if err != nil {
		return err
	}
	specPath, err := specPathFor(epicArg)
	if err != nil {
		return err
	}
	spec, err := os.ReadFile(filepath.Join(cwd, specPath))
	if err != nil {
		return err
	}
	built, err := ticketspec.BuildVerifyTickets(plan, string(spec), filepath.ToSlash(specPath))
	if err != nil {
		return err
	}
	if manifestPath != "" {
		if err := writeVerifyManifest(manifestPath, built); err != nil {
			return err
		}
	}
	for _, e := range built {
		fmt.Fprintf(w, "%s: window=%dK", e.Slug, e.ExpectedContextWindow/1000)
		if len(e.BlockedBy) > 0 {
			fmt.Fprintf(w, " blocked_by=%s", strings.Join(e.BlockedBy, ","))
		}
		if len(e.Stories) > 0 {
			fmt.Fprintf(w, " stories=%v", e.Stories)
		}
		if e.Note != "" {
			fmt.Fprintf(w, " (%s)", e.Note)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// writeVerifyManifest writes the body files next to the manifest, then appends
// the entries to it.
func writeVerifyManifest(path string, ts []ticketspec.VerifyTicket) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	out, err := ticketspec.AppendManifest(existing, ts)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	for _, t := range ts {
		if err := os.WriteFile(filepath.Join(dir, t.BodyFileName()), []byte(t.Body), 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(path, out, 0o644)
}
