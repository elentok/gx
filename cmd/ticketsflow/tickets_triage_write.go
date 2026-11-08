package ticketsflow

import (
	"errors"
	"fmt"
	"strings"
	"time"

	ticketscmd "github.com/elentok/gx/cmd/tickets"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/triage"

	"github.com/spf13/cobra"
)

// The write verbs check the real ticket first and skip what already happened,
// so a crash or a Retry never writes a row twice and no per-verb progress is
// stored in the record.

// Where a row's follow-up is now.
const (
	posFollowUps = "follow-ups"
	posMoved     = "moved"
	posArchived  = "archived"
	posMissing   = "missing"
)

// located is where a row's ticket is and what is already written into it.
type located struct {
	Position   string `json:"position"`
	Address    string `json:"address,omitempty"`
	Path       string `json:"path,omitempty"`
	HasAnswers bool   `json:"hasAnswers"`
	SeenAgain  bool   `json:"seenAgain"`

	body string
}

// locateRow finds the ticket first filed as the row's address: still in
// follow-ups, moved into another epic (original_address survives a move) or
// archived by close.
func locateRow(projectDir string, row *triage.Row) (located, error) {
	addr, err := tickets.ParseAddress(row.Address, tickets.AddressContext{})
	if err != nil {
		return located{}, err
	}
	epics, err := tickets.Load(projectDir)
	if err != nil {
		return located{}, err
	}
	var found *tickets.Ticket
	var foundEpic string
	for _, e := range epics {
		for i, t := range e.Tickets {
			atOrigin := e.Name == addr.Epic && t.Identifier == addr.ID && (t.OriginalAddress == "" || tickets.IsOriginal(t, addr))
			if atOrigin || tickets.IsOriginal(t, addr) {
				found, foundEpic = &e.Tickets[i], e.Name
			}
			if atOrigin {
				break
			}
		}
	}
	if found == nil {
		return located{Position: posMissing}, nil
	}
	loc := located{
		Position: posMoved,
		Address:  tickets.Address{Project: addr.Project, Epic: foundEpic, ID: found.Identifier}.String(),
		Path:     found.FilePath(),
		body:     found.Body,
	}
	switch foundEpic {
	case tickets.FollowUpsEpic:
		loc.Position = posFollowUps
	case tickets.ArchivedFollowUpsEpic:
		loc.Position = posArchived
	}
	lines := decisionLines(row)
	loc.HasAnswers = len(lines) == 0 || len(missingLines(schema.Section(found.Body, "Decisions"), lines)) == 0
	return loc, nil
}

// decisionLines is the person's answers as body lines, in question order.
func decisionLines(row *triage.Row) []string {
	var out []string
	for _, q := range row.AllQuestions() {
		if a, ok := row.Person.Answers[q.ID]; ok && strings.TrimSpace(a) != "" {
			out = append(out, fmt.Sprintf("- %s: %s", q.Text, strings.TrimSpace(a)))
		}
	}
	return out
}

func missingLines(section string, want []string) []string {
	have := map[string]bool{}
	for l := range strings.SplitSeq(section, "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var out []string
	for _, l := range want {
		if !have[l] {
			out = append(out, l)
		}
	}
	return out
}

// seenAgainLine is the dated entry for a merged row. Its "from <address>" part
// is the idempotency key, so the date does not matter on a rerun.
func seenAgainLine(from *triage.Row, now time.Time) string {
	line := fmt.Sprintf("- seen again %s from %s: %s", now.Format("2006-01-02"), from.Address, from.Title)
	if ev := strings.TrimSpace(from.Evidence); ev != "" {
		line += " — " + strings.Join(strings.Fields(ev), " ")
	}
	return line
}

func hasSeenAgain(evidence string, from *triage.Row) bool {
	for l := range strings.SplitSeq(evidence, "\n") {
		if strings.Contains(l, "seen again") && strings.Contains(l, " from "+from.Address+":") {
			return true
		}
	}
	return false
}

// appendToSection adds lines to the ticket's "## heading" section under the
// per-ticket lock.
func appendToSection(d Deps, path, heading string, lines []string) error {
	err := schema.UpdateTicketWithBody(path, func(_ *schema.Ticket, body *string) {
		cur := strings.TrimRight(schema.Section(*body, heading), "\n \t")
		if cur != "" {
			cur += "\n"
		}
		*body = schema.SetSection(*body, heading, cur+strings.Join(lines, "\n")+"\n")
	})
	if err != nil {
		return err
	}
	ticketscmd.PingServer(d.Env, path)
	return nil
}

// rowOf resolves the live record and one of its rows.
func rowOf(d Deps, project, address string) (triageTarget, *triage.Record, *triage.Row, error) {
	if address == "" {
		return triageTarget{}, nil, nil, errors.New("--row is required")
	}
	t, err := resolveTriage(d, project)
	if err != nil {
		return t, nil, nil, err
	}
	rec, err := t.live()
	if err != nil {
		return t, nil, nil, err
	}
	for i := range rec.Rows {
		if rec.Rows[i].Address == address {
			return t, rec, &rec.Rows[i], nil
		}
	}
	return t, rec, nil, fmt.Errorf("no row %s in triage %s", address, rec.ID)
}

func newTriageLocateCmd(d Deps) *cobra.Command {
	var project, row, seenFrom string
	cmd := &cobra.Command{
		Use:   "locate",
		Short: "say where a row's follow-up is now and what is already written into it",
		Long: "Usage: gx tickets triage locate --row <address> [--seen-from <address>] [--project <p>]\n\n" +
			"Print JSON: position (follow-ups, moved, archived or missing), the ticket's current address and\n" +
			"path, whether the person's answers are already in its Decisions, and, with --seen-from, whether\n" +
			"that merged row's \"seen again\" entry is already in its Evidence.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			t, rec, r, err := rowOf(d, project, row)
			if err != nil {
				return err
			}
			loc, err := locateRow(t.store.ProjectDir(), r)
			if err != nil {
				return err
			}
			if seenFrom != "" && loc.Position != posMissing {
				from, err := findRow(rec, seenFrom)
				if err != nil {
					return err
				}
				loc.SeenAgain = hasSeenAgain(schema.Section(loc.body, "Evidence"), from)
			}
			return writeJSON(c.OutOrStdout(), loc)
		},
	}
	f := cmd.Flags()
	f.StringVar(&project, "project", "", "project (default: the cwd's)")
	f.StringVar(&row, "row", "", "the row's follow-up address")
	f.StringVar(&seenFrom, "seen-from", "", "a merged row whose seen-again entry to look for")
	return cmd
}

func findRow(rec *triage.Record, address string) (*triage.Row, error) {
	for i := range rec.Rows {
		if rec.Rows[i].Address == address {
			return &rec.Rows[i], nil
		}
	}
	return nil, fmt.Errorf("no row %s in triage %s", address, rec.ID)
}

func newTriageAnswersCmd(d Deps) *cobra.Command {
	var project, row string
	cmd := &cobra.Command{
		Use:   "answers",
		Short: "write the person's answers into the ticket's Decisions (skips when already there)",
		Long: "Usage: gx tickets triage answers --row <address> [--project <p>]\n\n" +
			"Add the row's answers to the Decisions section of its ticket, wherever the ticket is now.\n" +
			"Lines already in the section are not added again. Prints \"written\" or \"already written\".",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			t, _, r, err := rowOf(d, project, row)
			if err != nil {
				return err
			}
			loc, err := locateRow(t.store.ProjectDir(), r)
			if err != nil {
				return err
			}
			if loc.Position == posMissing {
				return fmt.Errorf("no ticket found for %s", r.Address)
			}
			missing := missingLines(schema.Section(loc.body, "Decisions"), decisionLines(r))
			if len(missing) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "already written")
				return nil
			}
			if err := appendToSection(d, loc.Path, "Decisions", missing); err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), "written")
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project (default: the cwd's)")
	cmd.Flags().StringVar(&row, "row", "", "the row's follow-up address")
	return cmd
}

func newTriageSeenAgainCmd(d Deps) *cobra.Command {
	var project, row, from string
	cmd := &cobra.Command{
		Use:   "seen-again",
		Short: "add a dated \"seen again\" entry for a merged row to the kept ticket's Evidence",
		Long: "Usage: gx tickets triage seen-again --row <kept address> --from <merged address> [--project <p>]\n\n" +
			"Add the merged row's title and Evidence to the kept ticket's Evidence section. Skips when an entry\n" +
			"for that merged row is already there. Prints \"written\" or \"already written\".",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			t, rec, r, err := rowOf(d, project, row)
			if err != nil {
				return err
			}
			merged, err := findRow(rec, from)
			if err != nil {
				return err
			}
			loc, err := locateRow(t.store.ProjectDir(), r)
			if err != nil {
				return err
			}
			if loc.Position == posMissing {
				return fmt.Errorf("no ticket found for %s", r.Address)
			}
			if hasSeenAgain(schema.Section(loc.body, "Evidence"), merged) {
				fmt.Fprintln(c.OutOrStdout(), "already written")
				return nil
			}
			if err := appendToSection(d, loc.Path, "Evidence", []string{seenAgainLine(merged, time.Now())}); err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), "written")
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&project, "project", "", "project (default: the cwd's)")
	f.StringVar(&row, "row", "", "the kept row's follow-up address")
	f.StringVar(&from, "from", "", "the merged row's follow-up address")
	return cmd
}

func newTriageWriteStateCmd(d Deps) *cobra.Command {
	var project, row, state, newAddress, refusal string
	var revision int
	cmd := &cobra.Command{
		Use:   "write-state",
		Short: "save a row's write state: done, failed, gone or pending",
		Long: "Usage: gx tickets triage write-state --row <address> --state <state> [--new-address <a>]\n" +
			"  [--refusal <text>] [--revision <n>] [--project <p>]\n\n" +
			"Save the row's write state (step write only). --refusal is the failing verb's own text; any\n" +
			"state but failed clears it. Prints the new revision.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			t, rec, _, err := rowOf(d, project, row)
			if err != nil {
				return err
			}
			saved, err := t.store.WriteRow(rec.ID, currentRevision(c, rec, revision), row, triage.WriteState(state), newAddress, refusal)
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
	f.StringVar(&state, "state", "", "done, failed, gone or pending")
	f.StringVar(&newAddress, "new-address", "", "the ticket's address after a move")
	f.StringVar(&refusal, "refusal", "", "the refusing verb's own text (state failed)")
	f.IntVar(&revision, "revision", 0, "refuse unless the record is at this revision")
	return cmd
}

func newTriageEpicsCmd(d Deps) *cobra.Command {
	var project, ready string
	var maps []string
	var revision int
	cmd := &cobra.Command{
		Use:   "epics",
		Short: "record the epics this Triage creates, before `gx tickets epic add` runs",
		Long: "Usage: gx tickets triage epics [--ready <epic>] [--map <epic>]... [--revision <n>] [--project <p>]\n\n" +
			"Record the ready epic and map epics (step write only). Prints one line per epic: \"<epic> known\"\n" +
			"when the record listed it before this call, else \"<epic> new\". An `epic add` that refuses an\n" +
			"epic reported \"known\" is done; any other refusal fails the row with the verb's own text.",
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
			if _, err := t.store.RecordEpics(rec.ID, currentRevision(c, rec, revision), ready, maps); err != nil {
				return err
			}
			t.ping(d)
			out := c.OutOrStdout()
			for _, e := range append([]string{ready}, maps...) {
				if e == "" {
					continue
				}
				kind := "new"
				if rec.ListsEpic(e) {
					kind = "known"
				}
				fmt.Fprintf(out, "%s %s\n", e, kind)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&project, "project", "", "project (default: the cwd's)")
	f.StringVar(&ready, "ready", "", "the ready epic")
	f.StringArrayVar(&maps, "map", nil, "a map epic (repeatable)")
	f.IntVar(&revision, "revision", 0, "refuse unless the record is at this revision")
	return cmd
}

func newTriageLastPassCmd(d Deps) *cobra.Command {
	var project string
	var revision int
	var unset bool
	cmd := &cobra.Command{
		Use:   "last-pass",
		Short: "set the last-pass-complete flag; the record is done once every row is done, kept or gone",
		Long: "Usage: gx tickets triage last-pass [--unset] [--revision <n>] [--project <p>]\n\n" +
			"Set (or with --unset clear) the last-pass-complete flag (step write only). The record becomes done\n" +
			"when every row is done, kept or gone and the flag is set. Prints the new revision.",
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
			saved, err := t.store.SetLastPass(rec.ID, currentRevision(c, rec, revision), !unset)
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
	f.BoolVar(&unset, "unset", false, "clear the flag")
	f.IntVar(&revision, "revision", 0, "refuse unless the record is at this revision")
	return cmd
}
