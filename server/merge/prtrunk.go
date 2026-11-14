package merge

import (
	"fmt"
	"log/slog"
	"slices"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/ralphloop"
	rlmerge "github.com/elentok/gx/ralphloop/merge"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// SyncTrunkAfterMerge brings local Trunk up to the merged pull request: it fetches
// origin and fast-forwards Trunk under the Trunk lock. Only once Trunk holds the
// merge does it stamp merged_at, dequeue the root and send the merged notice. A
// failed update stores a pr-dirty-trunk or pr-trunk-update block (and its one
// notice, until the kind changes) and reports false; the caller retries.
func (s *Merger) SyncTrunkAfterMerge(root queue.RootRef) bool {
	return s.syncTrunk(root, true)
}

// syncTrunk is SyncTrunkAfterMerge; notify says whether success sends the merged
// notice. A hand Mark as merged sends none: the person did the merge.
func (s *Merger) syncTrunk(root queue.RootRef, notify bool) bool {
	dir, repo, err := s.projects.Of(root.Project)
	if err != nil {
		s.log.Warn("land merged PR: resolve project", "root", root, "err", err)
		return false
	}
	epics, err := tickets.LoadResolved(root.Project, dir)
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == root.Epic })
	if err != nil || i < 0 {
		s.log.Warn("land merged PR: load epic", "root", root, "err", err)
		return false
	}
	e := epics[i]
	one, _, target, err := s.EpicRefs(root.Project, epics, e.Name, repo)
	if err != nil {
		s.pauseTrunkUpdate(root, e, mergePause{kind: events.PRTrunkUpdate, reason: fmt.Sprintf("Trunk update failed: %v. Fix it, then merge again.", err)})
		return false
	}
	unlock := s.guards.Trunks.Lock(repo, target)
	p := fastForwardTrunk(repo, target)
	if p.kind != "" {
		unlock()
		s.pauseTrunkUpdate(root, e, p)
		return false
	}
	s.stampAndDequeueMerged(root, e, repo, target, unlock)
	if notify {
		s.notifyEpicOutcome(root.Project, dir, ralphloop.OneIteration{Epic: e.Name, Ticket: one}, e, true)
	}
	return true
}

// stampAndDequeueMerged is the shared tail of every merge that lands: under the
// Trunk lock the caller holds (released here) it stamps merged_at once, then
// clears the merge block, dequeues the root and publishes the events. An epic
// with no PR number (Mark as merged) gets the reflog range; a PR epic stamps no
// range here.
func (s *Merger) stampAndDequeueMerged(root queue.RootRef, e tickets.Epic, repo, target string, unlock func()) {
	// Read the stamp under the lock: a hand-merge detection racing us stamps first.
	if tickets.LoadEpicTicketMD(e.Path).MergedAt.IsZero() {
		added, deleted, cerr := rlmerge.EpicTrailerLines(repo, "refs/heads/"+e.Name, e.Name)
		s.stampMergedEpic(root, e, added, deleted, cerr)
		base, tip := "", ""
		if e.PRNumber == 0 {
			base, tip = reflogRange(s.log, root, e, repo, target)
		} else if !e.IsMap {
			base, tip = prRange(s.log, root, e, repo, target)
		}
		s.StampEpicRange(root, e.Path, base, tip, "merged_at")
	}
	unlock()
	s.clearMergeBlock(root, e.Path)
	removed, err := s.queue.RemoveRoot(root)
	if err != nil {
		s.log.Warn("dequeue merged root", "root", root, "err", err)
	}
	if removed {
		s.events.Publish(wire.EventQueueChanged, root.String())
		s.events.Publish(wire.EventRootCompleted, root.String())
	}
}

// prRange is the Epic range of a merged PR epic, by the merge style (see
// history.PRRange). GitHub's merge commit comes from the PR activity log: the
// Trunk update is retried with no live View, and a hand mark has none. Any
// refusal or git failure gives no range, logged, and never stops the stamp.
func prRange(log *slog.Logger, root queue.RootRef, e tickets.Epic, repo, target string) (base, tip string) {
	m := history.PRMerge{Number: e.PRNumber}
	entries, _ := prflow.Read(e.Path)
	for _, en := range entries { // newest first
		if en.Kind == prflow.KindPRMerged {
			m.MergeCommit = en.MergeCommit
			break
		}
	}
	tipRef := "refs/heads/" + target
	var err error
	if m.TrunkTip, err = git.RevParse(repo, tipRef); err != nil {
		log.Warn("read trunk tip, stamping no range", "root", root, "err", err)
		return "", ""
	}
	// A gone branch leaves no headlines: rule 2 is skipped.
	if branch := "refs/heads/" + e.Name; git.RefExists(repo, branch) {
		if m.ForkPoint, err = git.MergeBase(repo, branch, tipRef); err == nil {
			m.Headlines, err = history.Headlines(repo, m.ForkPoint+".."+branch)
		}
		if err != nil {
			log.Warn("read PR headlines, stamping no range", "root", root, "err", err)
			return "", ""
		}
	}
	r, reason, err := history.PRRange(repo, m)
	if err != nil {
		log.Warn("classify PR merge, stamping no range", "root", root, "err", err)
		return "", ""
	}
	if reason != "" {
		log.Info("no range for merged PR", "root", root, "reason", reason)
	}
	return r.Base, r.Tip
}

// pauseTrunkUpdate stores p, with its notice only when the stored kind differs:
// every tick retries, and one block must not notify every time.
func (s *Merger) pauseTrunkUpdate(root queue.RootRef, e tickets.Epic, p mergePause) {
	if fresh := tickets.LoadEpicAt(e.Path); fresh.MergeBlockKind == string(p.kind) {
		s.storeMergeBlock(root, e.Path, p)
		return
	}
	s.pauseEpic(root, e, p)
}

// fastForwardTrunk fetches origin and fast-forwards target's checkout to
// origin/target. A zero kind in the result means Trunk is up to date.
func fastForwardTrunk(repo, target string) mergePause {
	fail := func(format string, args ...any) mergePause {
		return mergePause{kind: events.PRTrunkUpdate, reason: fmt.Sprintf(format, args...)}
	}
	if err := git.Fetch(repo, "origin"); err != nil {
		return fail("Fetching origin failed: %v. Fix it, then merge again.", err)
	}
	remote := "refs/remotes/origin/" + target
	_, dir, err := checkoutDirFor(repo, target)
	if err != nil {
		return fail("Trunk update failed: %v. Fix it, then merge again.", err)
	}
	if held, err := git.IsAncestor(repo, remote, "refs/heads/"+target); err == nil && held {
		return mergePause{}
	}
	ok, err := history.MergeFastForward(dir, remote)
	if ok {
		return mergePause{}
	}
	if err == nil {
		return fail("%s has commits that are not on origin/%s, so it cannot fast-forward. Fix it, then merge again.", target, target)
	}
	if clashing := clashingBetween(repo, dir, "refs/heads/"+target, remote); len(clashing) > 0 {
		return mergePause{
			kind: events.PRDirtyTrunk,
			reason: fmt.Sprintf("uncommitted changes on %s (%s) clash with %s; commit or discard them, then merge again",
				target, dir, listFiles(clashing)),
			files: clashing,
		}
	}
	return fail("Trunk update failed: %v. Fix it, then merge again.", err)
}
