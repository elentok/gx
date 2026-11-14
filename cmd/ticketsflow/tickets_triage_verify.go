package ticketsflow

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/elentok/gx/triage"

	"github.com/spf13/cobra"
)

func newTriageVerdictCmd(d Deps) *cobra.Command {
	var (
		project, verdict, evidence, destination string
		row                                     string
		files, questions                        []string
		gone                                    bool
		revision                                int
	)
	cmd := &cobra.Command{
		Use:   "verdict",
		Short: "save one row's verdict (or --gone) and print the new revision",
		Long: "Usage: gx tickets triage verdict --row <address> [--verdict relevant|fixed|obsolete]\n" +
			"  [--evidence <text>] [--file <path>]... [--question <json>]... [--destination <epic>]\n" +
			"  [--gone] [--revision <n>] [--project <p>]\n\n" +
			"Save one row's verdict during Verify, or --gone when the follow-up is already closed or moved.\n" +
			"Each --question is a JSON object {\"text\", \"kind\", \"options\", \"default\"}: kind is decide, ask or\n" +
			"design; an ask question needs 2 to 4 options and a recommended default among them. Ids are q1, q2...\n" +
			"A stale --revision is refused; with none given the current revision is used.\n" +
			"The last verdict moves the stored step to Route.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if row == "" {
				return errors.New("--row is required")
			}
			t, err := resolveTriage(d, project)
			if err != nil {
				return err
			}
			rec, err := t.live()
			if err != nil {
				return err
			}
			if !c.Flags().Changed("revision") {
				revision = rec.Revision
			}
			v := triage.Verdict{
				Address:              row,
				Verdict:              verdict,
				Evidence:             evidence,
				Files:                files,
				SuggestedDestination: destination,
				Gone:                 gone,
			}
			for i, raw := range questions {
				var q triage.Question
				if err := json.Unmarshal([]byte(raw), &q); err != nil {
					return fmt.Errorf("--question %q is not a JSON question: %w", raw, err)
				}
				if q.ID == "" {
					q.ID = fmt.Sprintf("q%d", i+1)
				}
				q.RowID = row
				v.Questions = append(v.Questions, q)
			}
			saved, err := t.store.SaveVerdict(rec.ID, revision, v)
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
	f.StringVar(&verdict, "verdict", "", "relevant (default), fixed or obsolete")
	f.StringVar(&evidence, "evidence", "", "what the verdict rests on")
	f.StringVar(&destination, "destination", "", "suggested destination epic")
	f.StringArrayVar(&files, "file", nil, "a file the follow-up touches (repeatable)")
	f.StringArrayVar(&questions, "question", nil, "a JSON question {text, kind, options, default} (repeatable)")
	f.BoolVar(&gone, "gone", false, "the follow-up is already closed or moved")
	f.IntVar(&revision, "revision", 0, "refuse unless the record is at this revision")
	return cmd
}
