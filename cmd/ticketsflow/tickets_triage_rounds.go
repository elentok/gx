package ticketsflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/triage"

	"github.com/spf13/cobra"
)

// saveFlags are the flags every record-saving verb shares.
type saveFlags struct {
	project  string
	revision int
}

func (s *saveFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&s.project, "project", "", "project (default: the cwd's)")
	cmd.Flags().IntVar(&s.revision, "revision", 0, "refuse unless the record is at this revision")
}

// save runs op on the live record at --revision (else the current one), pings
// the server and prints the new revision.
func (s *saveFlags) save(c *cobra.Command, d Deps, op func(st *triage.Store, rec *triage.Record, revision int) (*triage.Record, error)) error {
	t, err := resolveTriage(d, s.project)
	if err != nil {
		return err
	}
	rec, err := t.live()
	if err != nil {
		return err
	}
	saved, err := op(t.store, rec, currentRevision(c, rec, s.revision))
	if err != nil {
		return err
	}
	t.ping(d)
	fmt.Fprintln(c.OutOrStdout(), saved.Revision)
	return nil
}

// parseQuestions reads a JSON array of questions.
func parseQuestions(r io.Reader) ([]triage.Question, error) {
	body, err := cmdbase.ReadBodyArg(r, "-")
	if err != nil {
		return nil, err
	}
	var qs []triage.Question
	if err := json.Unmarshal([]byte(body), &qs); err != nil {
		return nil, fmt.Errorf("questions on stdin: %w", err)
	}
	return qs, nil
}

func newTriageRoundCmd(d Deps) *cobra.Command {
	var sf saveFlags
	var objection bool
	cmd := &cobra.Command{
		Use:   "round",
		Short: "save a round of questions read as JSON on stdin and print the new revision",
		Long: "Usage: gx tickets triage round [--objection] [--revision <n>] [--project <p>]\n\n" +
			"Read a JSON array of questions on stdin: [{\"id\", \"rowId\", \"text\", \"kind\", \"options\", \"default\"}]\n" +
			"and save them as one round BEFORE printing it. Allowed at step questions. A round holds at most 6\n" +
			"ask or design questions; an ask question needs 2 to 4 options and a recommended default among them.\n" +
			"A fourth ordinary round is refused; --objection saves an objection round outside that cap.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			qs, err := parseQuestions(d.Stdin)
			if err != nil {
				return err
			}
			return sf.save(c, d, func(st *triage.Store, rec *triage.Record, rev int) (*triage.Record, error) {
				return st.SaveRound(rec.ID, rev, qs, objection)
			})
		},
	}
	sf.bind(cmd)
	cmd.Flags().BoolVar(&objection, "objection", false, "save an objection round (outside the 3-round cap)")
	return cmd
}

func newTriageAnswerCmd(d Deps) *cobra.Command {
	var sf saveFlags
	var question, answer string
	cmd := &cobra.Command{
		Use:   "answer",
		Short: "save the person's answer to one question of the open round",
		Long: "Usage: gx tickets triage answer --question <id> --answer <text> [--revision <n>] [--project <p>]\n\n" +
			"Save one answer to a question of the open round and print the new revision. The round is\n" +
			"answered when all its questions are.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if question == "" {
				return errors.New("--question is required")
			}
			return sf.save(c, d, func(st *triage.Store, rec *triage.Record, rev int) (*triage.Record, error) {
				return st.SaveAnswer(rec.ID, rev, question, answer)
			})
		},
	}
	sf.bind(cmd)
	cmd.Flags().StringVar(&question, "question", "", "the question's id")
	cmd.Flags().StringVar(&answer, "answer", "", "the person's answer")
	return cmd
}

// openRoundView is the open round: its index and the unanswered questions.
type openRoundView struct {
	Index     int               `json:"index"`
	Questions []triage.Question `json:"questions"`
}

func openRoundOf(rec *triage.Record) *openRoundView {
	if rec == nil {
		return nil
	}
	i, qs, ok := rec.OpenRound()
	if !ok {
		return nil
	}
	return &openRoundView{Index: i, Questions: qs}
}

func newTriageOpenRoundCmd(d Deps) *cobra.Command {
	var project string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "open-round",
		Short: "report the open round's unanswered questions",
		Long: "Usage: gx tickets triage open-round [--project <p>] [--json]\n\n" +
			"Report the first saved round with unanswered questions, so a fresh session reprints it and\n" +
			"skips what was answered. Prints nothing (JSON: null) when no round is open.",
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
			open := openRoundOf(rec)
			out := c.OutOrStdout()
			if asJSON {
				return writeJSON(out, open)
			}
			if open == nil {
				return nil
			}
			fmt.Fprintf(out, "round %d\n", open.Index+1)
			for _, q := range open.Questions {
				fmt.Fprintf(out, "%s  %s  %s  [%s]  recommended: %s\n", q.ID, q.RowID, q.Text, strings.Join(q.Options, " | "), q.Default)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project (default: the cwd's)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

func newTriageDecideCmd(d Deps) *cobra.Command {
	var sf saveFlags
	var row, question, choice, reason string
	cmd := &cobra.Command{
		Use:   "decide",
		Short: "save a question the agent settled itself",
		Long: "Usage: gx tickets triage decide --row <address> --question <text> --choice <text> [--reason <text>]\n" +
			"  [--revision <n>] [--project <p>]\n\n" +
			"Save a self-decided answer with its short reason. Allowed at step questions.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return sf.save(c, d, func(st *triage.Store, rec *triage.Record, rev int) (*triage.Record, error) {
				return st.SaveDecision(rec.ID, rev, triage.Decided{RowID: row, Question: question, Choice: choice, Reason: reason})
			})
		},
	}
	sf.bind(cmd)
	f := cmd.Flags()
	f.StringVar(&row, "row", "", "the row's follow-up address")
	f.StringVar(&question, "question", "", "the question the agent settled")
	f.StringVar(&choice, "choice", "", "the agent's choice")
	f.StringVar(&reason, "reason", "", "why")
	return cmd
}

func newTriageObjectCmd(d Deps) *cobra.Command {
	var sf saveFlags
	var row, question string
	var options []string
	cmd := &cobra.Command{
		Use:   "object",
		Short: "save the person's objection to a self-decided answer",
		Long: "Usage: gx tickets triage object --row <address> --question <text> --option <text>... [--revision <n>]\n" +
			"  [--project <p>]\n\n" +
			"Mark a decision as objected. It becomes an ask question in an open objection round, with the agent's\n" +
			"choice as the recommended answer; give 2 to 4 --option values that include that choice.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return sf.save(c, d, func(st *triage.Store, rec *triage.Record, rev int) (*triage.Record, error) {
				return st.Object(rec.ID, rev, row, question, options)
			})
		},
	}
	sf.bind(cmd)
	f := cmd.Flags()
	f.StringVar(&row, "row", "", "the row's follow-up address")
	f.StringVar(&question, "question", "", "the decision's question text")
	f.StringArrayVar(&options, "option", nil, "an answer to offer (repeatable)")
	return cmd
}

func newTriageCorrectCmd(d Deps) *cobra.Command {
	var sf saveFlags
	var row, action, destination, fallback string
	cmd := &cobra.Command{
		Use:   "correct",
		Short: "save a routing correction for a row",
		Long: "Usage: gx tickets triage correct --row <address> --action area|ready [--destination <area>]\n" +
			"  [--fallback <area>] [--revision <n>] [--project <p>]\n\n" +
			"Move a row to an area (--destination names it) or to ready (--fallback names the area it falls\n" +
			"back to). A move to an area drops the row's unanswered questions and keeps its answers.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return sf.save(c, d, func(st *triage.Store, rec *triage.Record, rev int) (*triage.Record, error) {
				return st.Correct(rec.ID, rev, row, action, destination, fallback)
			})
		},
	}
	sf.bind(cmd)
	f := cmd.Flags()
	f.StringVar(&row, "row", "", "the row's follow-up address")
	f.StringVar(&action, "action", "", "the new action: area or ready")
	f.StringVar(&destination, "destination", "", "the area to move to")
	f.StringVar(&fallback, "fallback", "", "the fallback area of a ready row")
	return cmd
}

func newTriageAreaCmd(d Deps) *cobra.Command {
	var sf saveFlags
	var name, source string
	var confirm bool
	cmd := &cobra.Command{
		Use:   "area",
		Short: "save an area entry, or confirm an agent area",
		Long: "Usage: gx tickets triage area --name <area> --source table|agent [--confirm] [--revision <n>]\n" +
			"  [--project <p>]\n\n" +
			"Save an area entry at step route or questions. An entry with a saved name replaces it. --confirm\n" +
			"records the person's confirmation of an agent area; approve is refused while one is unconfirmed.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return sf.save(c, d, func(st *triage.Store, rec *triage.Record, rev int) (*triage.Record, error) {
				return st.SaveAreas(rec.ID, rev, []triage.Area{{Name: name, Source: source, Confirmed: confirm}})
			})
		},
	}
	sf.bind(cmd)
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "the area epic's name")
	f.StringVar(&source, "source", "", "table or agent")
	f.BoolVar(&confirm, "confirm", false, "the person confirmed this area")
	return cmd
}
