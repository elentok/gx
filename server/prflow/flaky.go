package prflow

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets"
)

func hasEntry(entries []Entry, kind Kind, sha, key string) bool {
	return slices.ContainsFunc(entries, func(e Entry) bool {
		return e.Kind == kind && e.HeadSHA == sha && slices.Contains(e.Checks, key)
	})
}

// runCompleted reports whether every check of the workflow run has finished;
// GitHub only reruns a job of a completed run.
func runCompleted(checks []pr.Check, runID string) bool {
	for _, c := range checks {
		if c.RunID == runID && !c.Completed() {
			return false
		}
	}
	return true
}

// rerun reruns c's job. The entry is logged first so a crash or a restart never
// reruns the same (check, head) twice.
func (w *Watcher) rerun(r pr.Runner, repo string, t Target, v View, c pr.Check, key string) error {
	job, err := strconv.ParseInt(c.JobID, 10, 64)
	if err != nil {
		return fmt.Errorf("job id of check %s: %w", key, err)
	}
	if err := Append(t.Dir, Entry{Kind: KindReranCheck, Checks: []string{key}, HeadSHA: v.HeadSHA}); err != nil {
		return err
	}
	return r.RerunJob(repo, job)
}

// settleReruns marks every rerun of this head that now passes as flaky, once,
// and files its follow-up.
func (w *Watcher) settleReruns(t Target, v View, entries []Entry) error {
	for _, e := range entries {
		if e.Kind != KindReranCheck || e.HeadSHA != v.HeadSHA || len(e.Checks) != 1 {
			continue
		}
		key := e.Checks[0]
		if hasEntry(entries, KindFlaky, v.HeadSHA, key) {
			continue
		}
		i := slices.IndexFunc(v.HeadChecks, func(c pr.Check) bool { return CheckKeyString(c.Key()) == key })
		if i < 0 || !v.HeadChecks[i].Green() {
			continue
		}
		if err := Append(t.Dir, Entry{Kind: KindFlaky, Checks: []string{key}, HeadSHA: v.HeadSHA}); err != nil {
			return err
		}
		c := v.HeadChecks[i]
		url := c.URL
		if url == "" {
			url = t.URL
		}
		from := tickets.Address{Project: t.Project, Epic: t.Epic}
		err := w.host.FileFlakyFollowUp(t.Project, from, ticketwrite.FollowUp{
			Slug:         "flaky-check",
			Title:        "Flaky CI check " + key,
			Source:       fmt.Sprintf("The PR flow of %s:%s reran failed check %s once and the rerun passed.", t.Project, t.Epic, key),
			Problem:      fmt.Sprintf("Check: %s\nRun URL: %s\nHead SHA: %s", key, url, v.HeadSHA),
			SuggestedFix: "Find why the check fails intermittently and fix or quarantine it.",
			Key:          key,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
