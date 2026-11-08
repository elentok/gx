package ticketsflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/elentok/gx/cmd/ticketref"
	"github.com/elentok/gx/triage"

	"github.com/spf13/cobra"
)

const triagePingTimeout = time.Second

// triageTarget is the store and project name a triage verb works on.
type triageTarget struct {
	store   *triage.Store
	project string
}

func resolveTriage(d Deps, project string) (triageTarget, error) {
	root, name, err := ticketref.ResolveProject(d.Env, project)
	if err != nil {
		return triageTarget{}, err
	}
	return triageTarget{store: triage.NewStore(root), project: name}, nil
}

// ping tells a running server the record changed. Best effort: the record is
// already written, so a down server is silent.
func (t triageTarget) ping(d Deps) {
	cl, err := d.Env.Dial()()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), triagePingTimeout)
	defer cancel()
	_ = cl.Triage.Changed(ctx, t.project)
}

// live returns the live record or an error saying there is none.
func (t triageTarget) live() (*triage.Record, error) {
	rec, err := t.store.LoadLive()
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, errors.New("no live Triage in " + t.project)
	}
	return rec, nil
}

// latest is the live record, else the newest finished one, else nil.
func (t triageTarget) latest() (*triage.Record, error) {
	rec, err := t.store.LoadLive()
	if err != nil || rec != nil {
		return rec, err
	}
	hist, err := t.store.History()
	if err != nil || len(hist) == 0 {
		return nil, err
	}
	return hist[0], nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func newTicketsTriageCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "triage",
		Short: "work with the Triage record of a project's follow-ups",
	}
	cmd.AddCommand(newTriageCreateCmd(d), newTriageShowCmd(d), newTriagePendingCmd(d), newTriageVerdictCmd(d),
		newTriagePlanCmd(d), newTriageRowCmd(d), newTriageApproveCmd(d), newTriageAbandonCmd(d))
	return cmd
}

func newTriageCreateCmd(d Deps) *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "start a Triage record and print its id (refuses while one is live)",
		Long:  "Usage: gx tickets triage create [--project <p>]\n\nStart a Triage record and print its id. Refuses, naming the live record, when one exists.",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			t, err := resolveTriage(d, project)
			if err != nil {
				return err
			}
			rec, err := t.store.Create()
			if err != nil {
				return err
			}
			t.ping(d)
			fmt.Fprintln(c.OutOrStdout(), rec.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project (default: the cwd's)")
	return cmd
}

// triageView is the show output: the record plus the step to take next.
type triageView struct {
	*triage.Record
	Next triage.Branch `json:"next"`
}

func newTriageShowCmd(d Deps) *cobra.Command {
	var project string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "show the live Triage record (or the latest) with its next step",
		Long: "Usage: gx tickets triage show [--project <p>] [--json]\n\n" +
			"Show the live record, else the latest, with its revision, the detected next step\n" +
			"(start, verify, plan, approve, write or done) and the rows.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			t, err := resolveTriage(d, project)
			if err != nil {
				return err
			}
			rec, err := t.latest()
			if err != nil {
				return err
			}
			view := triageView{Record: rec, Next: triage.Next(rec)}
			out := c.OutOrStdout()
			if asJSON {
				return writeJSON(out, view)
			}
			if rec == nil {
				fmt.Fprintf(out, "no Triage; next: %s\n", view.Next)
				return nil
			}
			fmt.Fprintf(out, "%s  state=%s step=%s revision=%d next=%s\n", rec.ID, rec.State, rec.Step, rec.Revision, view.Next)
			for _, r := range rec.Rows {
				fmt.Fprintf(out, "  %s  verdict=%s write=%s  %s\n", r.Address, r.Verdict, r.Write, r.Title)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project (default: the cwd's)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

func newTriagePendingCmd(d Deps) *cobra.Command {
	var project string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "pending",
		Short: "list the live record's rows with no verdict that are not gone",
		Long: "Usage: gx tickets triage pending [--project <p>] [--json]\n\n" +
			"List the rows with no verdict and not gone, so a fresh Verify session checks only those.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			t, err := resolveTriage(d, project)
			if err != nil {
				return err
			}
			rec, err := t.live()
			if err != nil {
				return err
			}
			rows := rec.PendingVerdicts()
			out := c.OutOrStdout()
			if asJSON {
				if rows == nil {
					rows = []triage.Row{}
				}
				return writeJSON(out, rows)
			}
			for _, r := range rows {
				fmt.Fprintf(out, "%s  %s\n", r.Address, r.Title)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project (default: the cwd's)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}
