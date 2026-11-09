package ticketsflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/triage"

	"github.com/spf13/cobra"
)

// currentRevision is the revision a verb works from: --revision when given,
// else the live record's.
func currentRevision(c *cobra.Command, rec *triage.Record, revision int) int {
	if c.Flags().Changed("revision") {
		return revision
	}
	return rec.Revision
}

func newTriagePlanCmd(d Deps) *cobra.Command {
	var project string
	var gone []string
	var revision int
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "save the plan rows read as JSON on stdin and print the new revision",
		Long: "Usage: gx tickets triage plan [--gone <address>]... [--revision <n>] [--project <p>]\n\n" +
			"Read a JSON array of plan rows on stdin: [{\"rowId\", \"action\", \"destination\", \"questions\":\n" +
			"[{\"id\", \"text\", \"kind\", \"options\", \"default\"}], \"overlapWith\"}]. A quick question needs 2 to 4\n" +
			"options and a default that is one of them (the recommended one); a design question needs neither. Every\n" +
			"verdict question comes back here under its id, worded as the final question. Rows named by --gone are\n" +
			"marked gone and their plan rows are left out. When every other row has a plan the stored step moves to approve.",
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
			body, err := cmdbase.ReadBodyArg(d.Stdin, "-")
			if err != nil {
				return err
			}
			var plan []triage.PlanRow
			if err := json.Unmarshal([]byte(body), &plan); err != nil {
				return fmt.Errorf("plan rows on stdin: %w", err)
			}
			saved, err := t.store.SavePlan(rec.ID, currentRevision(c, rec, revision), plan, gone)
			if err != nil {
				return err
			}
			t.ping(d)
			fmt.Fprintln(c.OutOrStdout(), saved.Revision)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&project, "project", "", "project (default: the cwd's)")
	f.StringArrayVar(&gone, "gone", nil, "a row found already closed or moved (repeatable)")
	f.IntVar(&revision, "revision", 0, "refuse unless the record is at this revision")
	return cmd
}

func newTriageRowCmd(d Deps) *cobra.Command {
	var (
		project, row, action, destination string
		answers                           []string
		keep                              bool
		revision                          int
	)
	cmd := &cobra.Command{
		Use:   "row",
		Short: "save a person's fields for one row (step approve only)",
		Long: "Usage: gx tickets triage row --row <address> --revision <n> [--action <a>] [--destination <epic>]\n" +
			"  [--answer <question-id>=<text>]... [--keep-for-later] [--project <p>]\n\n" +
			"Replace the person's fields for one row. --revision is required and a stale one is refused.\n" +
			"Allowed only at step approve.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if row == "" {
				return errors.New("--row is required")
			}
			if !c.Flags().Changed("revision") {
				return errors.New("--revision is required")
			}
			f := triage.PersonFields{Action: action, Destination: destination, KeepForLater: keep}
			for _, a := range answers {
				id, text, ok := strings.Cut(a, "=")
				if !ok || id == "" {
					return fmt.Errorf("--answer %q is not <question-id>=<text>", a)
				}
				if f.Answers == nil {
					f.Answers = map[string]string{}
				}
				f.Answers[id] = text
			}
			t, err := resolveTriage(d, project)
			if err != nil {
				return err
			}
			rec, err := t.live()
			if err != nil {
				return err
			}
			saved, err := t.store.SetPerson(rec.ID, revision, row, f)
			if err != nil {
				return err
			}
			t.ping(d)
			fmt.Fprintln(c.OutOrStdout(), saved.Revision)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&project, "project", "", "project (default: the cwd's)")
	f.StringVar(&row, "row", "", "the row's follow-up address")
	f.StringVar(&action, "action", "", "the person's action")
	f.StringVar(&destination, "destination", "", "the person's destination")
	f.StringArrayVar(&answers, "answer", nil, "an answer, <question-id>=<text> (repeatable)")
	f.BoolVar(&keep, "keep-for-later", false, "keep the follow-up where it is")
	f.IntVar(&revision, "revision", 0, "required: refuse unless the record is at this revision")
	return cmd
}

func newTriageApproveCmd(d Deps) *cobra.Command {
	var project string
	var revision int
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "approve the plan at the given revision; prints the new revision and write",
		Long: "Usage: gx tickets triage approve --revision <n> [--project <p>]\n\n" +
			"Move the step from approve to write if the record is still at --revision. Prints the new\n" +
			"revision and \"write\". A step already past approve prints \"already approved\"; a changed\n" +
			"revision at step approve is refused as stale. Refused while a question is open or a merge\n" +
			"target is orphaned.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !c.Flags().Changed("revision") {
				return errors.New("--revision is required")
			}
			t, err := resolveTriage(d, project)
			if err != nil {
				return err
			}
			rec, err := t.live()
			if err != nil {
				return err
			}
			saved, err := t.store.Approve(rec.ID, revision)
			if err != nil {
				return err
			}
			t.ping(d)
			fmt.Fprintf(c.OutOrStdout(), "%d write\n", saved.Revision)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project (default: the cwd's)")
	cmd.Flags().IntVar(&revision, "revision", 0, "required: the revision the person approved")
	return cmd
}

func newTriageAbandonCmd(d Deps) *cobra.Command {
	var project string
	var revision int
	cmd := &cobra.Command{
		Use:   "abandon",
		Short: "abandon the live Triage (refused once the step is write)",
		Long: "Usage: gx tickets triage abandon [--revision <n>] [--project <p>]\n\n" +
			"End the live record without writing to tickets. Allowed at verify, plan and approve.",
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
			saved, err := t.store.Abandon(rec.ID, currentRevision(c, rec, revision))
			if err != nil {
				return err
			}
			t.ping(d)
			fmt.Fprintln(c.OutOrStdout(), saved.Revision)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project (default: the cwd's)")
	cmd.Flags().IntVar(&revision, "revision", 0, "refuse unless the record is at this revision")
	return cmd
}
