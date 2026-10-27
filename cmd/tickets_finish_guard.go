package cmd

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets/schema"
	"github.com/spf13/cobra"
)

// checkFinishedGuard refuses `--iteration-status finished` for a ticket whose
// iteration branch has commits unless `gx check` recorded a run of the
// worktree's current tree. The refusal is a plain error: it writes nothing and
// never pauses (ADR 0042). A ticket with no iteration branch or worktree, or a
// branch with no commits, skips the guard.
func checkFinishedGuard(c *cobra.Command, path string, getwd func() (string, error)) error {
	if v, _ := c.Flags().GetString("iteration-status"); schema.IterationStatus(v) != schema.IterationStatusFinished {
		return nil
	}
	issuesDir := filepath.Dir(path)
	if filepath.Base(issuesDir) != "issues" {
		return nil
	}
	epicDir := filepath.Dir(issuesDir)
	epic := filepath.Base(epicDir)
	ticket, err := schema.ParseTicket(path)
	if err != nil {
		return nil
	}
	pf, err := config.ReadProjectFile(filepath.Dir(epicDir))
	if err != nil || pf.Repo == nil || *pf.Repo == "" {
		return nil
	}
	repo, err := git.FindRepo(*pf.Repo)
	if err != nil {
		return nil
	}
	worktrees, err := git.ListWorktrees(*repo)
	if err != nil {
		return nil
	}
	branch := ralphloop.IterationBranch(epic, string(ticket.ID))
	wt := git.WorktreePathForBranch(branch, worktrees)
	if wt == "" {
		return nil
	}
	// Counted from HEAD: an agent may have detached it.
	base, err := git.MergeBase(wt, "HEAD", epic)
	if err != nil {
		return nil
	}
	if ahead, err := git.CommitsAhead(wt, base, "HEAD"); err != nil || ahead == 0 {
		return nil
	}

	tree, err := git.TreeID(wt)
	if err != nil {
		return fmt.Errorf("%s: reading the tree of %s: %w", ticketLabel(path), wt, err)
	}
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	recs, err := check.ReadRecords(dataDir, check.ProjectFor(wt))
	if err != nil {
		return err
	}
	recs = slices.DeleteFunc(check.FilterRecords(recs, branch, tree), func(r check.Record) bool {
		return r.KindFilter != ""
	})

	failures, _ := c.Flags().GetString("check-failures")
	var addrs []string
	for _, a := range strings.Split(failures, ",") {
		if a = strings.TrimSpace(a); a != "" && !slices.Contains(addrs, a) {
			addrs = append(addrs, a)
		}
	}

	var failing *check.Record
	for i := range recs {
		switch recs[i].Exit {
		case check.ExitPass, check.ExitNone:
			return nil
		case check.ExitFail:
			failing = &recs[i]
		}
	}
	label := ticketLabel(path)
	if failing == nil {
		return fmt.Errorf("%s: no passing `gx check` run for the current tree of %s; run `gx check` and fix it, or file follow-ups for flakes and pre-existing failures and finish with --check-failures <follow-up addrs>", label, branch)
	}

	need := max(1, len(failedTests(*failing)))
	if len(addrs) < need {
		return fmt.Errorf("%s: the last `gx check` run failed %d test(s); pass --check-failures with at least %d follow-up address(es), got %d", label, need, need, len(addrs))
	}
	cwd, err := getwd()
	if err != nil {
		return err
	}
	for _, a := range addrs {
		if _, _, err := findTicket(cwd, a); err != nil {
			return fmt.Errorf("%s: --check-failures %s: %w", label, a, err)
		}
	}
	return nil
}

func failedTests(r check.Record) []string {
	var names []string
	for _, k := range r.Kinds {
		for _, n := range k.Failed {
			if !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
	}
	return names
}
