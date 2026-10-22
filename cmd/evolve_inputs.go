package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/spf13/cobra"
)

// Flag names in the evolve inputs table. They are part of the JSON contract the
// gx-evolve skill reads.
const (
	flagPark          = "park"
	flagNeedsRepair   = "needs-repair"
	flagRecovery      = "recovery"
	flagDeadlock      = "deadlock"
	flagConflict      = "conflict-hit"
	flagRelaunch      = "relaunch"
	flagNudge         = "nudge"
	flagOverrun       = "context-overrun"
	flagSlow          = "slow"
	flagMetricsAbsent = "metrics-missing"
	flagFixTickets    = "has-fix-tickets"
)

// Severity tiers for the deep-read cap, highest first.
const (
	severityRest     = 0
	severityOverrun  = 1
	severityConflict = 2
	severityParked   = 3
)

// InputsRow is one ticket of the flag table.
type InputsRow struct {
	ID               string         `json:"id"`
	Type             string         `json:"type"`
	Parent           string         `json:"parent,omitempty"`
	Status           string         `json:"status"`
	ExpectedContext  int            `json:"expected_context"`
	ActualContext    int            `json:"actual_context"`
	ElapsedSeconds   int            `json:"elapsed_seconds"`
	Cost             float64        `json:"cost"`
	Sessions         []string       `json:"sessions,omitempty"`
	ConflictSessions []string       `json:"conflict_sessions,omitempty"`
	Events           map[string]int `json:"events,omitempty"`
	// Landed is true when the epic's branch carries this ticket's
	// Ralph-Loop-Ticket trailer.
	Landed     bool     `json:"landed"`
	FixTickets []string `json:"fix_tickets,omitempty"`
	Flags      []string `json:"flags"`
	Severity   int      `json:"severity"`
	// Digests lists the session digests written into the table's DigestDir.
	Digests []InputsDigest `json:"digests,omitempty"`
}

// InputsTable is what `gx evolve inputs` reports for one epic.
type InputsTable struct {
	Epic string `json:"epic"`
	// MedianElapsedSeconds is the baseline the slow flag compares against.
	MedianElapsedSeconds int `json:"median_elapsed_seconds"`
	// Deadlocks counts epic-wide deadlock events, which carry no ticket.
	Deadlocks int `json:"deadlocks"`
	// DigestDir holds the text-only transcript digests the rows point at.
	DigestDir string      `json:"digest_dir"`
	Rows      []InputsRow `json:"rows"`
}

// buildInputsTable derives every row from frontmatter, the run log and the
// landed set; none of it needs a transcript. landed may be nil (no repo).
func buildInputsTable(t evolveTarget, evs []ralphloop.Event, landed map[string]bool) (InputsTable, error) {
	e := t.epic
	forkParents := e.ForkParents()

	byTicket := map[string][]ralphloop.Event{}
	deadlocks := 0
	for _, ev := range evs {
		if ev.Type == string(events.Deadlocked) {
			deadlocks++
		}
		if ev.Ticket != "" {
			byTicket[ev.Ticket] = append(byTicket[ev.Ticket], ev)
		}
	}

	// A code-review ticket's fix tickets are its children, minus the forks
	// the loop itself opens (conflict resolution, investigation).
	fixes := map[string][]string{}
	for _, tk := range e.Tickets {
		p, ok := forkParents.Of(tk)
		if !ok || !p.IsCodeReview() {
			continue
		}
		if tk.Type == string(schema.TypeConflictResolution) || tk.Type == string(schema.TypeInvestigate) {
			continue
		}
		fixes[p.Identifier] = append(fixes[p.Identifier], tk.Identifier)
	}

	var elapsed []int
	for _, tk := range e.Tickets {
		if tk.ElapsedTime > 0 {
			elapsed = append(elapsed, tk.ElapsedTime)
		}
	}
	median := medianInt(elapsed)

	table := InputsTable{Epic: t.project + ":" + t.name, MedianElapsedSeconds: median, Deadlocks: deadlocks}
	for _, tk := range e.Tickets {
		sc, err := schema.ParseTicket(tk.Path)
		if err != nil {
			return InputsTable{}, fmt.Errorf("ticket %s: %w", tk.Identifier, err)
		}
		row := InputsRow{
			ID:              tk.Identifier,
			Type:            tk.Type,
			Status:          tk.Status,
			ExpectedContext: tk.ExpectedContextWindow,
			ActualContext:   tk.ActualContextWindow,
			ElapsedSeconds:  tk.ElapsedTime,
			Cost:            tk.ActualCost,
			Landed:          landed[tk.Identifier],
			FixTickets:      fixes[tk.Identifier],
			Flags:           []string{},
		}
		if tk.Parent != nil {
			row.Parent = *tk.Parent
		}
		row.Sessions, row.ConflictSessions, row.Events = digestTicketEvents(sc.SessionIDs, byTicket[tk.Identifier])
		row.Flags = inputsFlags(tk, row, median)
		row.Severity = inputsSeverity(row.Flags)
		table.Rows = append(table.Rows, row)
	}

	sort.SliceStable(table.Rows, func(i, j int) bool { return table.Rows[i].Severity > table.Rows[j].Severity })
	return table, nil
}

// digestTicketEvents folds a ticket's run-log events into its session ids
// (frontmatter first, then iteration starts), its conflict-resolution
// sessions and per-type event counts.
func digestTicketEvents(frontmatter []string, evs []ralphloop.Event) (sessions, conflict []string, counts map[string]int) {
	add := func(list []string, id string) []string {
		if id == "" || slices.Contains(list, id) {
			return list
		}
		return append(list, id)
	}
	for _, id := range frontmatter {
		sessions = add(sessions, id)
	}
	for _, ev := range evs {
		if counts == nil {
			counts = map[string]int{}
		}
		counts[ev.Type]++
		switch ev.Type {
		case string(events.IterationStarted):
			sessions = add(sessions, ev.AgentSession)
		case string(events.ConflictHit), string(events.ConflictResolved):
			conflict = add(conflict, ev.AgentSession)
		}
	}
	return sessions, conflict, counts
}

func inputsFlags(tk tickets.Ticket, row InputsRow, median int) []string {
	has := func(prefix string) bool {
		for typ, n := range row.Events {
			if n > 0 && strings.HasPrefix(typ, prefix) {
				return true
			}
		}
		return false
	}
	flags := []string{}
	add := func(cond bool, f string) {
		if cond {
			flags = append(flags, f)
		}
	}
	add(has(string(events.NeedsAnswer)) || tk.LegacyStatus() == string(schema.StatusNeedsAnswer), flagPark)
	add(has(string(events.NeedsRepair)) || tk.LegacyStatus() == string(schema.StatusNeedsRepair), flagNeedsRepair)
	add(has("recovery-"), flagRecovery)
	add(has(string(events.Deadlocked)), flagDeadlock)
	add(has(string(events.ConflictHit)), flagConflict)
	// Conflict sessions are listed apart, so any extra session here is a relaunch.
	add(len(row.Sessions) > 1, flagRelaunch)
	add(has(string(events.Nudged)), flagNudge)
	add(row.ExpectedContext > 0 && row.ActualContext > 2*row.ExpectedContext, flagOverrun)
	add(median > 0 && row.ElapsedSeconds > 2*median, flagSlow)
	add(tk.Status == string(schema.StatusDone) && !tk.Commitless && (row.ActualContext == 0 || row.ElapsedSeconds == 0), flagMetricsAbsent)
	add(len(row.FixTickets) > 0, flagFixTickets)
	return flags
}

// inputsSeverity ranks a flag set: parked or broken beats conflict beats
// overrun beats the rest.
func inputsSeverity(flags []string) int {
	sev := severityRest
	for _, f := range flags {
		switch f {
		case flagPark, flagNeedsRepair, flagRecovery, flagDeadlock:
			return severityParked
		case flagConflict:
			sev = max(sev, severityConflict)
		case flagOverrun:
			sev = max(sev, severityOverrun)
		}
	}
	return sev
}

func medianInt(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

// runEvolveInputs prints the epic's flag table, as JSON when asked.
func runEvolveInputs(env evolveEnv, w io.Writer, arg string, asJSON bool) error {
	t, err := resolveEvolveTarget(env, arg)
	if err != nil {
		return err
	}
	evs, _, err := ralphloop.ReadEvents(filepath.Dir(t.epic.Path), t.name)
	if err != nil {
		return err
	}
	var landed map[string]bool
	if env.landed != nil {
		// An unreadable repo only costs the landed column.
		if landed, err = env.landed(t.project, t.name); err != nil {
			landed = nil
		}
	}
	table, err := buildInputsTable(t, evs, landed)
	if err != nil {
		return err
	}
	if table.DigestDir, err = os.MkdirTemp(env.tempDir, "gx-evolve-digests-"); err != nil {
		return err
	}
	if err := writeDigests(env, t, table.DigestDir, &table); err != nil {
		return err
	}
	// The JSON is a machine contract, so the reminder stays out of it.
	if asJSON {
		return writeJSON(w, table)
	}
	if err := printInputsTable(w, table); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "digests: %s\n", table.DigestDir); err != nil {
		return err
	}
	return evolveReminder(env, t.mode, w)
}

func printInputsTable(w io.Writer, table InputsTable) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTYPE\tPARENT\tSTATUS\tEXP\tACT\tELAPSED\tCOST\tSESSIONS\tEVENTS\tFLAGS\tSEV")
	for _, r := range table.Rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%ds\t%.2f\t%s\t%s\t%s\t%d\n",
			r.ID, r.Type, dash(r.Parent), r.Status, r.ExpectedContext, r.ActualContext,
			r.ElapsedSeconds, r.Cost, dash(strings.Join(r.Sessions, ",")),
			dash(formatEventCounts(r.Events)), dash(strings.Join(r.Flags, ",")), r.Severity)
	}
	return tw.Flush()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatEventCounts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, m[k])
	}
	return strings.Join(parts, " ")
}

// defaultLanded reads the epic's Ralph-Loop-Ticket trailers off its branch.
// A project with no repo reports nothing landed, not an error.
func defaultLanded(storePath string) func(project, epic string) (map[string]bool, error) {
	return func(project, epic string) (map[string]bool, error) {
		dir, err := tickets.ProjectDirByName(storePath, project)
		if err != nil {
			return nil, err
		}
		pf, err := config.ReadProjectFile(dir)
		if err != nil || pf.Repo == nil {
			return nil, err
		}
		repo, err := git.FindRepo(*pf.Repo)
		if err != nil {
			return nil, err
		}
		trailers, err := git.TrailerMap(repo.Root, epic, ralphloop.TicketTrailerKey)
		if err != nil {
			return nil, err
		}
		landed := map[string]bool{}
		for value := range trailers {
			if e, id, ok := tickets.SplitTrailerValue(value); ok && e == epic {
				landed[id] = true
			}
		}
		return landed, nil
	}
}

func newEvolveInputsCmd(env func() (evolveEnv, error)) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "inputs <epic> [--json]",
		Short: "print the epic's per-ticket flag table",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return runEvolveInputs(e, c.OutOrStdout(), args[0], asJSON)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit structured JSON instead of a table")
	return c
}
