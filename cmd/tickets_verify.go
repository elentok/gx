package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"text/tabwriter"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/spf13/cobra"
)

// verifyResult is the --json payload of `gx tickets verify`: always the full,
// unfiltered list, with a landing in flight reported beside it.
type verifyResult struct {
	Tickets         []ralphloop.TicketVerification `json:"tickets"`
	LandingInFlight *ralphloop.LandMarker          `json:"landing_in_flight"`
}

// verifyRun is everything runTicketsVerify needs besides its flags.
type verifyRun struct {
	EpicPath        string
	ID              string // empty verifies the whole epic
	FeatureWorktree string
	WorktreeDir     string
	WorkspaceID     string
	Deps            ralphloop.VerifyDeps
}

func newTicketsVerifyCmd(d deps) *cobra.Command {
	var jsonOut, all bool
	cmd := &cobra.Command{
		Use:   "verify <epic> [id]",
		Short: "report whether each ticket's commits landed on the feature branch (read-only)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			run, err := defaultVerifyRun(resolveEpicArg(args[0], cwd), cwd)
			if err != nil {
				return finishRecovery(c.OutOrStdout(), c.ErrOrStderr(), jsonOut, nil, "", err)
			}
			if len(args) == 2 {
				run.ID = args[1]
			}
			return runTicketsVerify(run, all, jsonOut, c.OutOrStdout(), c.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the full unfiltered list as JSON")
	cmd.Flags().BoolVar(&all, "all", false, "show every ticket, not just those needing attention")
	return cmd
}

// defaultVerifyRun wires the real git/herdr dependencies. Herdr is optional:
// without a workspace the tab leftovers are reported as unknown.
func defaultVerifyRun(epicPath, cwd string) (verifyRun, error) {
	repo, err := git.FindRepo(cwd)
	if err != nil {
		return verifyRun{}, fmt.Errorf("not inside a git repo: %w", err)
	}
	epicPath = filepath.Clean(epicPath)
	vd := ralphloop.DefaultVerifyDeps()
	run := verifyRun{
		EpicPath:        epicPath,
		FeatureWorktree: filepath.Join(repo.WorktreeDir, filepath.Base(epicPath)),
		WorktreeDir:     repo.WorktreeDir,
		Deps:            vd,
	}
	if ws, err := herdr.FindWorkspace(filepath.Base(epicPath)); err == nil {
		run.WorkspaceID = ws
	} else {
		run.Deps.TabList = nil
	}
	return run, nil
}

// runTicketsVerify is write-free and takes no land lock: reading owns nothing,
// so it works while a land or a live tab is in flight.
func runTicketsVerify(run verifyRun, all, jsonMode bool, stdout, stderr io.Writer) error {
	result, err := gatherVerify(run)
	if err != nil {
		return finishRecovery(stdout, stderr, jsonMode, nil, "", err)
	}
	if jsonMode {
		return finishRecovery(stdout, stderr, true, result, "", nil)
	}
	printVerifyTable(stdout, result, all)
	return nil
}

func gatherVerify(run verifyRun) (verifyResult, error) {
	epicPath := filepath.Clean(run.EpicPath)
	epicName := filepath.Base(epicPath)
	epics, err := tickets.Load(filepath.Dir(epicPath))
	if err != nil {
		return verifyResult{}, fmt.Errorf("loading epics under %s: %w", filepath.Dir(epicPath), err)
	}
	var epic *tickets.Epic
	for i := range epics {
		if epics[i].Name == epicName {
			epic = &epics[i]
		}
	}
	if epic == nil {
		return verifyResult{}, fmt.Errorf("epic not found: %s", epicPath)
	}

	selected := epic.Tickets
	if run.ID != "" {
		selected = nil
		for _, t := range epic.Tickets {
			if t.DisplayNumber() == run.ID {
				selected = append(selected, t)
			}
		}
		if len(selected) == 0 {
			return verifyResult{}, fmt.Errorf("ticket %s not found in epic %s", run.ID, epicName)
		}
	}

	events, _, err := ralphloop.ReadEvents(filepath.Dir(epicPath), epicName)
	if err != nil {
		return verifyResult{}, fmt.Errorf("reading run log: %w", err)
	}
	verifications, err := ralphloop.VerifyEpic(run.Deps, ralphloop.VerifyParams{
		Epic:            epicName,
		FeatureWorktree: run.FeatureWorktree,
		WorktreeDir:     run.WorktreeDir,
		WorkspaceID:     run.WorkspaceID,
		Tickets:         selected,
		Events:          events,
	})
	if err != nil {
		return verifyResult{}, err
	}
	if verifications == nil {
		verifications = []ralphloop.TicketVerification{}
	}

	marker, err := ralphloop.ReadLandMarker(epicPath)
	if err != nil {
		return verifyResult{}, fmt.Errorf("reading land marker: %w", err)
	}
	if marker != nil && marker.Epic != epicName {
		marker = nil
	}
	return verifyResult{Tickets: verifications, LandingInFlight: marker}, nil
}

// needsAttention is the human table's filter: anything not landed, plus
// landed tickets whose status never reached done.
func needsAttention(v ralphloop.TicketVerification) bool {
	return v.Landing != ralphloop.LandingLanded || v.Status != "done"
}

func printVerifyTable(w io.Writer, r verifyResult, all bool) {
	if m := r.LandingInFlight; m != nil {
		fmt.Fprintf(w, "landing in flight: ticket %s (conflict pending, range %s)\n\n", m.Ticket, m.SourceRange)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tLANDING\tEVIDENCE\tLEFTOVERS")
	hidden := 0
	for _, v := range r.Tickets {
		if !all && !needsAttention(v) {
			hidden++
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", v.ID, v.Status, v.Landing, v.Evidence, leftoversText(v.Leftovers))
	}
	tw.Flush()
	if !all {
		fmt.Fprintf(w, "%d hidden (landed and done); use --all to show them\n", hidden)
	}
}

// leftoversText lists the leftovers that exist; "?" marks a check that got no
// answer (herdr down), which is not the same as absent.
func leftoversText(l ralphloop.Leftovers) string {
	out := ""
	add := func(name string, b *bool) {
		switch {
		case b == nil:
			name += "?"
		case !*b:
			return
		}
		if out != "" {
			out += ","
		}
		out += name
	}
	add("tab", l.Tab)
	add("worktree", l.Worktree)
	add("branch", l.Branch)
	if out == "" {
		return "-"
	}
	return out
}
