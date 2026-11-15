package ticketsflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/triage"

	"github.com/spf13/cobra"
)

// moveLocated is what a move unit still has to do, read from the real store.
type moveLocated struct {
	// Exists is false once the old epic is archived or gone.
	Exists bool `json:"exists"`
	// Move lists the open and draft tickets still in the old epic.
	Move    []string `json:"move"`
	Claimed []string `json:"claimed"`
	Stamped bool     `json:"stamped"`
	// Archive is true when the epic is still there, nothing is left to move and
	// no claimed ticket stops the archive. Run it after the move and the stamp.
	Archive bool `json:"archive"`
}

func locateMove(projectDir, project, epic string) (moveLocated, error) {
	epics, err := tickets.Load(projectDir)
	if err != nil {
		return moveLocated{}, err
	}
	out := moveLocated{Move: []string{}, Claimed: []string{}}
	for _, e := range epics {
		if e.Name != epic {
			continue
		}
		out.Exists = true
		out.Stamped = !e.CompletedAt.IsZero()
		c := triage.ClassifyOldEpic(project, e)
		out.Move, out.Claimed = c.Move, c.Claimed
		out.Archive = len(c.Move) == 0 && c.CanArchive()
	}
	return out, nil
}

func newTriageMovesCmd(d Deps) *cobra.Command {
	var project string
	var revision int
	cmd := &cobra.Command{
		Use:   "moves",
		Short: "save the old-epic move units read as JSON on stdin and print the new revision",
		Long: "Usage: gx tickets triage moves [--revision <n>] [--project <p>]\n\n" +
			"Read a JSON array on stdin: [{\"oldEpic\", \"backlog\"}] (step route only). Each unit is saved pending and\n" +
			"replaces the earlier list. Leave out an epic that holds a claimed ticket and nothing to move: the\n" +
			"summary shows it as waiting. The backlog must be an epic of the project or a backlog of this triage.",
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
			var units []triage.MoveUnit
			if err := json.Unmarshal([]byte(body), &units); err != nil {
				return fmt.Errorf("move units on stdin: %w", err)
			}
			for _, u := range units {
				loc, err := locateMove(t.store.ProjectDir(), t.project, u.OldEpic)
				if err != nil {
					return err
				}
				if loc.Exists && len(loc.Move) == 0 && len(loc.Claimed) > 0 {
					return fmt.Errorf("old epic %s holds claimed %s and nothing to move: leave it out, it is waiting", u.OldEpic, strings.Join(loc.Claimed, ", "))
				}
			}
			saved, err := t.store.SaveMoves(rec.ID, currentRevision(c, rec, revision), units)
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
	f.IntVar(&revision, "revision", 0, "refuse unless the record is at this revision")
	return cmd
}

func newTriageMoveLocateCmd(d Deps) *cobra.Command {
	var project, epic string
	cmd := &cobra.Command{
		Use:   "move-locate",
		Short: "say what a move unit still has to do, read from the real store",
		Long: "Usage: gx tickets triage move-locate --epic <old epic> [--project <p>]\n\n" +
			"Print JSON: exists (false once archived), the open and draft tickets still to move, claimed tickets,\n" +
			"whether completed_at is stamped, and archive (true when nothing is left to move and none is claimed).\n" +
			"Run `gx tickets move` for each ticket in move, then, when archive is true, stamp and archive when the\n" +
			"epic is not stamped or archived yet. A rerun skips what already happened.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if epic == "" {
				return errors.New("--epic is required")
			}
			t, err := resolveTriage(d, project)
			if err != nil {
				return err
			}
			loc, err := locateMove(t.store.ProjectDir(), t.project, epic)
			if err != nil {
				return err
			}
			return writeJSON(c.OutOrStdout(), loc)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project (default: the cwd's)")
	cmd.Flags().StringVar(&epic, "epic", "", "the old epic")
	return cmd
}

func newTriageMoveStateCmd(d Deps) *cobra.Command {
	var project, epic, state, lastError string
	var revision int
	cmd := &cobra.Command{
		Use:   "move-state",
		Short: "save a move unit's state: pending, done, failed or kept",
		Long: "Usage: gx tickets triage move-state --epic <old epic> --state <state> [--error <text>]\n" +
			"  [--revision <n>] [--project <p>]\n\n" +
			"Save the unit's state (step write only). --error is the failing verb's own text; any state but\n" +
			"failed clears it. Prints the new revision.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if epic == "" {
				return errors.New("--epic is required")
			}
			t, err := resolveTriage(d, project)
			if err != nil {
				return err
			}
			rec, err := t.live()
			if err != nil {
				return err
			}
			saved, err := t.store.SaveMoveState(rec.ID, currentRevision(c, rec, revision), epic, triage.MoveState(state), lastError)
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
	f.StringVar(&epic, "epic", "", "the old epic")
	f.StringVar(&state, "state", "", "pending, done, failed or kept")
	f.StringVar(&lastError, "error", "", "the failing verb's own text (state failed)")
	f.IntVar(&revision, "revision", 0, "refuse unless the record is at this revision")
	return cmd
}
