package prflow

import (
	"slices"

	"github.com/elentok/gx/tickets"
)

// closeOrMerge ends the watching of a PR GitHub reports MERGED or CLOSED. A
// deleted remote branch is reported CLOSED, so it takes the closed path.
func (w *Watcher) closeOrMerge(t Target, v View) (handled bool, err error) {
	switch v.State {
	case "MERGED":
		return true, w.Merged(t)
	case "CLOSED":
		return true, w.closed(t)
	}
	return false, nil
}

// Merged runs the merged path of t's PR: the one "PR merged" entry, the end of
// the flow's tickets, then the Trunk update. It is also what an adopted merged
// PR calls. The state comes from the PR, never from ancestry: a squash merge
// leaves the head SHA off Trunk.
func (w *Watcher) Merged(t Target) error {
	if !logged(t.Dir, KindPRMerged) {
		// Before the entry, so a crash retries the cancel along with the update.
		w.host.CancelFlowTickets(t)
		if err := tickets.ClearEpicFields(t.Dir, blockKindKey, blockReasonKey); err != nil {
			return err
		}
		if err := Append(t.Dir, Entry{Kind: KindPRMerged, Detail: t.URL}); err != nil {
			return err
		}
	}
	return w.updateTrunk(t)
}

// updateTrunk is the retried half of the merged path: it runs every tick, and
// writes "trunk updated" once Trunk holds the merge.
func (w *Watcher) updateTrunk(t Target) error {
	if !w.host.SyncTrunkAfterMerge(t) {
		return nil
	}
	return Append(t.Dir, Entry{Kind: KindTrunkUpdated})
}

// retryMerged retries the Trunk update of every epic whose "PR merged" entry is
// written and whose merged_at is not stamped. Such a PR is no longer polled.
func (w *Watcher) retryMerged() {
	for _, t := range w.host.Watched() {
		// Watched reads the index, which may not show a stamp this tick just made.
		if logged(t.Dir, KindPRMerged) && tickets.LoadEpicTicketMD(t.Dir).MergedAt.IsZero() {
			_ = w.updateTrunk(t)
		}
	}
}

// closed clears the epic's PR identity, so it is Awaiting PR again, and logs
// it. Auto-PR skips it because "PR closed" is newer than "PR opened".
func (w *Watcher) closed(t Target) error {
	// The index lags the disk, so a second pass can still see the closed PR.
	if tickets.LoadEpicAt(t.Dir).PRNumber == 0 {
		return nil
	}
	if err := tickets.ClearEpicFields(t.Dir, "pr_number", "pr_url"); err != nil {
		return err
	}
	return Append(t.Dir, Entry{Kind: KindPRClosed, Detail: t.URL})
}

func logged(dir string, k Kind) bool {
	entries, err := Read(dir)
	return err == nil && slices.ContainsFunc(entries, func(e Entry) bool { return e.Kind == k })
}
