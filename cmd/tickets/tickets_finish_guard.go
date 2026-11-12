package tickets

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/cmd/ticketref"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/tickets"
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
	branch := worktree.IterationBranch(epic, string(ticket.ID))
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
		return fmt.Errorf("%s: reading the tree of %s: %w", ticketref.TicketLabel(path), wt, err)
	}
	dataDir, err := dirs.DataDir()
	if err != nil {
		return err
	}
	recs, err := check.ReadRecords(dataDir, check.ProjectForDirs(dirs, wt))
	if err != nil {
		return err
	}
	recs0 := recs
	recs = slices.DeleteFunc(check.FilterRecords(recs, branch, tree), func(r check.Record) bool {
		return r.KindFilter != ""
	})

	addrs := splitAddrs(c, "check-failures")

	// A skip ("no applicable checks") is the result of committing after a
	// failed run: it must not replace the failure, so it only counts when no
	// run of this tree failed.
	var failing *check.Record
	skipped := false
	for i := range recs {
		switch recs[i].Exit {
		case check.ExitPass:
			return nil
		case check.ExitNone:
			skipped = true
		case check.ExitFail:
			failing = &recs[i]
		}
	}
	if failing == nil && skipped {
		return nil
	}
	label := ticketref.TicketLabel(path)
	if failing == nil {
		return fmt.Errorf("%s: no passing `gx check` run for the current tree of %s%s; run `gx check` and fix it, or file follow-ups for flakes and pre-existing failures and finish with --check-failures <follow-up addrs>", label, branch, staleRunHint(recs0, branch))
	}

	// Tests already red on the base are waived: a ticket claimed on a red tree
	// can still finish. Only a failure the base did not have needs a follow-up.
	failedNow := failedTests(*failing)
	preexisting := baselineFailures(recs0, base)
	var fresh []string
	for _, n := range failedNow {
		if slices.Contains(preexisting, n) {
			fmt.Fprintf(c.ErrOrStderr(), "pre-existing failure (red on the base, waived): %s\n", n)
		} else {
			fresh = append(fresh, n)
		}
	}
	if len(failedNow) > 0 && len(fresh) == 0 {
		return nil
	}

	// A known failure covers the whole failing set: one follow-up that tracks
	// a pre-existing breakage (e.g. a broken test family) waives the one-per-test
	// count, so a suite that is red on the base does not block every ticket.
	known := splitAddrs(c, "known-failures")
	need := max(1, len(fresh))
	if len(known) > 0 {
		addrs, need = known, len(known)
	}
	if len(addrs) < need {
		hint := ""
		if need > 1 {
			hint = "; if they are already red on the base, file one follow-up for the family and pass --known-failures <addr> instead"
		}
		hint += knownFlakeHint(epicDir, failedTests(*failing))
		return fmt.Errorf("%s: the last `gx check` run failed %d test(s); pass --check-failures with at least %d follow-up address(es), got %d%s", label, need, need, len(addrs), hint)
	}
	cwd, err := getwd()
	if err != nil {
		return err
	}
	for _, a := range addrs {
		if _, _, err := ticketref.FindTicket(dirs, cwd, a); err != nil {
			return fmt.Errorf("%s: --check-failures %s: %w", label, a, err)
		}
	}
	return nil
}

// staleRunHint explains a refusal when the branch has full-run records for
// other trees: a pass for an older tree means a later commit made it stale,
// and a skip as the latest record means the agent must pass --commitless false.
func staleRunHint(recs []check.Record, branch string) string {
	var branchRecs []check.Record
	for _, r := range check.FilterRecords(recs, branch, "") {
		if r.KindFilter == "" {
			branchRecs = append(branchRecs, r)
		}
	}
	if len(branchRecs) == 0 {
		return ""
	}
	hint := ""
	for _, r := range slices.Backward(branchRecs) {
		if r.Exit == check.ExitPass {
			tree := r.TreeID
			if len(tree) > 7 {
				tree = tree[:7]
			}
			hint = fmt.Sprintf(" (the passing run of tree %s at %s is stale: a new commit changed the tree; rerun `gx check`)", tree, r.Time)
			break
		}
	}
	if branchRecs[len(branchRecs)-1].Exit == check.ExitNone {
		hint += " (the last `gx check` skipped with no applicable checks; pass --commitless false)"
	}
	return hint
}

// baselineFailures is the failing set of the latest full failed `gx check`
// run whose HEAD was the base commit: what was already red before the ticket.
func baselineFailures(recs []check.Record, base string) []string {
	for _, r := range slices.Backward(recs) {
		if r.Head == base && r.Exit == check.ExitFail && r.KindFilter == "" {
			return failedTests(r)
		}
	}
	return nil
}

// knownFlakeHint names the open follow-ups whose title or Problem mentions a
// failed test (the full name, or its parent when it is a subtest), so an agent
// passes the address instead of re-diagnosing a failure that is already filed.
func knownFlakeHint(epicDir string, failed []string) string {
	projectDir := filepath.Dir(epicDir)
	epic := tickets.LoadEpic(projectDir, tickets.FollowUpsEpic)
	var b strings.Builder
	for _, name := range failed {
		needles := []string{name}
		if parent, _, ok := strings.Cut(name, "/"); ok {
			needles = append(needles, parent)
		}
		for _, t := range epic.Tickets {
			if t.IsDone() || t.IsCancelled() {
				continue
			}
			haystack := t.Title + "\n" + schema.Section(t.Body, "Problem")
			if slices.ContainsFunc(needles, func(n string) bool { return strings.Contains(haystack, n) }) {
				a := tickets.Address{Project: filepath.Base(projectDir), Epic: tickets.FollowUpsEpic, ID: t.Identifier}
				fmt.Fprintf(&b, "\nknown flake: %s (%s)", a, name)
			}
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String() + "\nrerun `gx check` once, then pass the address via --check-failures (or --known-failures for a family red on the base)"
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
