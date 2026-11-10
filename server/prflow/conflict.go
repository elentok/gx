package prflow

import (
	"fmt"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// landBlocked is true while the epic stores a block only a person's Merge again
// clears: "needs rebase" or "foreign commits conflict".
func landBlocked(epicDir string) bool {
	switch tickets.LoadEpicAt(epicDir).MergeBlockKind {
	case string(schema.PauseNeedsRebase), string(schema.PausePRForeignConflict):
		return true
	}
	return false
}

// startConflictFix handles the epic's open PR conflicting with Trunk and a
// finished push ticket that hit foreign commits. stop is true while the epic is
// blocked, so no further fix starts. `UNKNOWN` forks nothing, and `BEHIND` is
// not a conflict.
func (w *Watcher) startConflictFix(t Target, v View) (stop bool, err error) {
	if v.State != "OPEN" {
		return false, nil
	}
	if landBlocked(t.Dir) {
		return true, nil
	}
	if reason := foreignConflictReason(t.Dir); reason != "" {
		return true, w.host.PauseForeignConflict(t, reason)
	}
	if v.Mergeable != "CONFLICTING" {
		return false, nil
	}
	return false, w.host.ForkPRMerge(queue.RootRef{Project: t.Project, Epic: t.Epic})
}

// foreignConflictReason is why the newest ci-fix or merge ticket stopped: it is
// done and its Resolution says `Outcome: foreign-conflict`. A newer open ticket
// (after Merge again) hides an older one's outcome.
func foreignConflictReason(epicDir string) string {
	var newest tickets.Ticket
	for _, t := range tickets.LoadEpicAt(epicDir).Tickets {
		if (t.Type == string(schema.TypeCIFix) || t.Type == string(schema.TypeMerge)) && t.Number >= newest.Number {
			newest = t
		}
	}
	if newest.Path == "" || newest.Status != string(schema.StatusDone) {
		return ""
	}
	if out, err := OutcomeOf(newest.Path); err != nil || out != OutcomeForeignConflict {
		return ""
	}
	return fmt.Sprintf("Someone else pushed commits to this branch that conflict with the rebase (ticket %s). Resolve it by hand, then merge again.", newest.Identifier)
}
