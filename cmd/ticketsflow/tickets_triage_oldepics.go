package ticketsflow

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/skills"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/triage"
	"github.com/spf13/cobra"
)

const oldEpicAreasFile = "gx-triage-follow-ups/old-epic-areas.txt"

// oldEpic is one old triage-made area map epic the first Triage after the
// change moves into areas.
type oldEpic struct {
	Epic string `json:"epic"`
	// Area is the shipped table's area; empty when the epic is not in the table.
	Area string `json:"area"`
	// Move lists the open and draft tickets to move.
	Move []string `json:"move"`
	// Archive is true when no open, draft or claimed ticket would be left.
	Archive bool `json:"archive"`
	// Waiting is true when there is nothing to move and a claimed ticket stops
	// the archive; Claimed names those tickets.
	Waiting bool     `json:"waiting"`
	Claimed []string `json:"claimed"`
}

// shippedOldEpicAreas reads the table that ships with the skill:
// project -> old epic -> area.
func shippedOldEpicAreas() (map[string]map[string]string, error) {
	data, err := skills.Bundle.ReadFile(oldEpicAreasFile)
	if err != nil {
		return nil, err
	}
	table := map[string]map[string]string{}
	for line := range strings.SplitSeq(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if len(f) != 3 {
			return nil, fmt.Errorf("%s: want <project> <epic> <area>, got %q", oldEpicAreasFile, line)
		}
		if table[f[0]] == nil {
			table[f[0]] = map[string]string{}
		}
		table[f[0]][f[1]] = f[2]
	}
	return table, nil
}

func listOldEpics(projectDir, project string, table map[string]map[string]string) ([]oldEpic, error) {
	epics, err := tickets.Load(projectDir)
	if err != nil {
		return nil, err
	}
	out := []oldEpic{}
	for _, e := range epics {
		if !triage.IsOldEpic(e) {
			continue
		}
		c := oldEpic{Epic: e.Name, Area: table[project][e.Name], Move: []string{}, Claimed: []string{}}
		k := triage.ClassifyOldEpic(project, e)
		c.Move, c.Claimed = k.Move, k.Claimed
		c.Archive = k.CanArchive()
		c.Waiting = k.Waiting()
		out = append(out, c)
	}
	return out, nil
}

func newTriageOldEpicsCmd(d Deps) *cobra.Command {
	var project string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "oldepics",
		Short: "list the old triage-made area epics the first Triage must move into areas",
		Long: "Usage: gx tickets triage oldepics [--project <p>] [--json]\n\n" +
			"List the charting, unarchived map epics whose Notes say \"Triaged from follow-ups\", each with its\n" +
			"open and draft tickets to move, its area from the shipped table (empty when not in it), whether it\n" +
			"can archive after the move, and, when nothing can move and a claimed ticket stays, a waiting flag.\n" +
			"Prints an empty list when there are none. Read only.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			t, err := resolveTriage(d, project)
			if err != nil {
				return err
			}
			table, err := shippedOldEpicAreas()
			if err != nil {
				return err
			}
			list, err := listOldEpics(t.store.ProjectDir(), t.project, table)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if asJSON {
				return writeJSON(out, list)
			}
			for _, e := range list {
				switch {
				case e.Waiting:
					fmt.Fprintf(out, "waiting: %s holds claimed %s\n", e.Epic, strings.Join(e.Claimed, ", "))
				default:
					fmt.Fprintf(out, "%s  area=%s move=%d archive=%t\n", e.Epic, e.Area, len(e.Move), e.Archive)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project (default: the cwd's)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}
