package cmd

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets/schema"
	"github.com/spf13/cobra"
)

// checkFinishedGuard refuses `--iteration-status finished` for a ticket whose
// iteration branch has commits unless `gx check` recorded a run of the
// worktree's current tree. The refusal is a plain error: it writes nothing and
// never pauses (ADR 0042). A ticket with no iteration branch or worktree, or a
// branch with no commits, skips the guard.
func checkFinishedGuard(dirs config.Dirs, c *cobra.Command, path string, getwd func() (string, error)) error {
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
	wt := history.WorktreePathForBranch(branch, worktrees)
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

	tree, err := history.TreeID(wt)
	if err != nil {
		return fmt.Errorf("%s: reading the tree of %s: %w", ticketLabel(path), wt, err)
	}
	dataDir, err := dirs.DataDir()
	if err != nil {
		return err
	}
	recs, err := check.ReadRecords(dataDir, check.ProjectForDirs(dirs, wt))
	if err != nil {
		return err
	}
	recs = slices.DeleteFunc(check.FilterRecords(recs, branch, tree), func(r check.Record) bool {
		return r.KindFilter != ""
	})

	addrs := splitAddrs(c, "check-failures")

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

	// A known failure covers the whole failing set: one follow-up that tracks
	// a pre-existing breakage (e.g. a broken test family) waives the one-per-test
	// count, so a suite that is red on the base does not block every ticket.
	known := splitAddrs(c, "known-failures")
	need := max(1, len(failedTests(*failing)))
	if len(known) > 0 {
		addrs, need = known, len(known)
	}
	if len(addrs) < need {
		return fmt.Errorf("%s: the last `gx check` run failed %d test(s); pass --check-failures with at least %d follow-up address(es), got %d", label, need, need, len(addrs))
	}
	cwd, err := getwd()
	if err != nil {
		return err
	}
	for _, a := range addrs {
		if _, _, err := findTicket(dirs, cwd, a); err != nil {
			return fmt.Errorf("%s: --check-failures %s: %w", label, a, err)
		}
	}
	return nil
}

// splitAddrs reads a comma-separated address flag, trimmed and de-duplicated.
func splitAddrs(c *cobra.Command, flag string) []string {
	v, _ := c.Flags().GetString(flag)
	var addrs []string
	for _, a := range strings.Split(v, ",") {
		if a = strings.TrimSpace(a); a != "" && !slices.Contains(addrs, a) {
			addrs = append(addrs, a)
		}
	}
	return addrs
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
