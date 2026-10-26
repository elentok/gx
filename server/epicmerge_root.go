package server

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// completeRootIfDone merges the root's feature branch once every ticket in its
// epic is done, then dequeues it. A branch that needs a rebase gets a merge
// ticket instead, or pauses the root at the cap. With the project's auto-ff-merge off,
// and no manual merge ticket in the epic, the
// branch is left alone: the root is dequeued as complete and a person runs gx-merge.
// completed_at marks the epic's final outcome: with auto-ff-merge on that is
// the merge or the pause, not all-done. An epic with nothing to merge is merged
// as it stands, whatever auto-ff-merge says.
func (s *Server) completeRootIfDone(root rootRef, one ralphloop.OneIteration) {
	// The one-offs epic is a standing inbox: it never merges, merges or retros.
	if root.Epic == OneOffsEpic {
		return
	}
	project := root.Project
	dir, repo, err := s.projectOf(project)
	if err != nil {
		s.log.Warn("complete root", "root", root, "err", err)
		return
	}
	epics, err := tickets.LoadResolved(project, dir)
	if err != nil {
		s.log.Warn("complete root", "root", root, "err", err)
		return
	}
	for _, e := range epics {
		if filepath.Base(e.Path) == one.Epic && ralphloop.AllDone(e) {
			s.completeEpic(root, one, dir, repo, epics, e)
		}
	}
}

// completeEpic is completeRootIfDone for the all-done epic e. The merge decides
// under its trunk's lock, so a second merge into that trunk sees the first one's
// result; notices and the retro go after unlock, so claiming elsewhere never
// waits on them.
//
// Without refs there is no target, so the merge locks the epic itself: mergeEpic
// pauses it the way mergeRoot does, and two finishes still can't both stamp.
func (s *Server) completeEpic(root rootRef, one ralphloop.OneIteration, dir, repo string, epics []tickets.Epic, e tickets.Epic) {
	_, _, target, err := s.epicRefs(root.Project, epics, one.Epic, repo)
	lockKey := target
	if err != nil {
		lockKey = "epic:" + root.String()
	}
	unlock := s.trunkLocks.lock(repo, lockKey)
	d := s.mergeEpic(root, one, repo, epics, e, target, false)
	unlock()
	s.actOnMerge(root, one, dir, e, d)
}

// mergeOutcome is how a merge ended; the zero value is a refusal with nothing
// to act on.
type mergeOutcome int

const (
	outcomeNone mergeOutcome = iota
	// outcomeMerged: the epic merged (or had nothing to merge) and was dequeued.
	outcomeMerged
	// outcomeCompleted: all-done with auto-ff-merge off; the branch is left alone.
	outcomeCompleted
	outcomePaused
	// outcomeMergeTicket: a non-ff forked a merge ticket; the root stays queued.
	outcomeMergeTicket
	// outcomeAlreadyMerged: merged elsewhere and already dequeued.
	outcomeAlreadyMerged
)

// mergeDecision is what mergeEpic decided under the lock, for its caller to act
// on after unlock. first is set only for the caller that made the
// completed_at or merged_at stamp transition: that keeps the retro and the
// epic notice once-only without holding the lock around them.
type mergeDecision struct {
	result  QueueResult
	outcome mergeOutcome
	first   bool
}

// completed reports whether the root left the queue as a final outcome.
func (d mergeDecision) completed() bool {
	return d.outcome == outcomeMerged || d.outcome == outcomeCompleted
}

// sendNotice: a merge always speaks, as an epic merged elsewhere notifies with
// its dequeue. A pause with a kind waits on a person's fix, so its chat notice
// goes every time; a needs-rebase pause and a completion are the epic's outcome
// and speak only when their stamp is new.
func (d mergeDecision) sendNotice() bool {
	switch d.outcome {
	case outcomeMerged:
		return true
	case outcomeCompleted:
		return d.first
	case outcomePaused:
		return d.result.PauseKind != string(events.NeedsRebase) || d.first
	}
	return false
}

func (d mergeDecision) submitRetro() bool {
	switch d.outcome {
	case outcomeMerged, outcomeCompleted, outcomePaused, outcomeAlreadyMerged:
		return d.first
	}
	return false
}

func pauseDecision(first bool, reason string, kind events.Kind) mergeDecision {
	return mergeDecision{
		result:  QueueResult{Paused: true, PauseKind: string(kind), Message: reason},
		outcome: outcomePaused,
		first:   first,
	}
}

// mergeEpic is completeEpic's work under target's trunk lock: stamps, git and
// the dequeue. force merges with auto-ff-merge off: the manual trigger's direct
// merge.
func (s *Server) mergeEpic(root rootRef, one ralphloop.OneIteration, repo string, epics []tickets.Epic, e tickets.Epic, target string, force bool) mergeDecision {
	project := root.Project
	// A stored block is the last merge's; this merge decides afresh, and a new
	// pause writes it again.
	s.clearMergeBlock(root, e.Path)
	fresh := tickets.LoadEpicTicketMD(e.Path)
	already := !fresh.MergedAt.IsZero()
	first := fresh.CompletedAt.IsZero()
	merged := true
	switch {
	case already:
		if first {
			s.stampEpic(root, e.Path, "completed_at")
		}
	case s.nothingToMerge(project, epics, one, repo):
		s.stampMergedEpic(root, e, 0, 0, nil)
		s.stampEpic(root, e.Path, "merged_at", "completed_at")
		first = true
	case force || s.autoFFMerge(project) || hasManualMergeTicket(e):
		trunkBefore, revErr := git.RevParse(repo, "refs/heads/"+target)
		pause := s.mergeRoot(project, epics, one, repo)
		// A non-ff below the cap is not an outcome: the merge ticket makes the
		// epic not all-done, so the root stays queued and its finish merges again.
		// The ticket is never manual, even under force: it counts against the cap.
		if pause.reason != "" && pause.kind == events.NeedsRebase {
			if id := s.forkMergeTicket(root, e, target, false); id != "" {
				return mergeDecision{result: QueueResult{MergeTicket: tickets.Address{Project: project, Epic: root.Epic, ID: id}.String()}, outcome: outcomeMergeTicket}
			}
		}
		if pause.reason != "" {
			s.stampEpic(root, e.Path, "completed_at")
			s.storeMergeBlock(root, e.Path, pause)
			return pauseDecision(first, pause.reason, pause.kind)
		}
		var added, removed int
		countErr := revErr
		if countErr == nil {
			added, removed, countErr = git.DiffLines(repo, trunkBefore, "refs/heads/"+target)
		}
		s.stampMergedEpic(root, e, added, removed, countErr)
		s.stampEpic(root, e.Path, "merged_at", "completed_at")
		first = true
	default:
		s.stampEpic(root, e.Path, "completed_at")
		merged = false
	}
	d := mergeDecision{result: QueueResult{Merged: merged}, outcome: outcomeCompleted, first: first}
	if merged {
		d.outcome = outcomeMerged
	}
	removed, err := s.queued.removeRoot(root)
	if err != nil {
		s.log.Warn("dequeue completed root", "root", root, "err", err)
	} else if removed {
		s.events.publish(EventQueueChanged, root.String())
	}
	// An epic merged elsewhere notifies with the dequeue, so a double merge sends one.
	if already && !removed {
		d.outcome = outcomeAlreadyMerged
	}
	return d
}

// actOnMerge sends d's notices and retro. It runs after the trunk lock is
// released, and returns d's result.
func (s *Server) actOnMerge(root rootRef, one ralphloop.OneIteration, dir string, e tickets.Epic, d mergeDecision) QueueResult {
	if d.result.Paused {
		s.pauseRoot(root, one, dir, e, d)
	} else if d.sendNotice() {
		s.notifyEpicOutcome(root.Project, dir, one, e, d.result.Merged)
	}
	if d.submitRetro() {
		s.submitRetro(root, dir, e)
	}
	// Last, so a watcher of root-completed sees the retro already filed.
	if d.completed() {
		s.events.publish(EventRootCompleted, root.String())
	}
	return d.result
}

// pauseRoot reports the pause of e's all-done root.
func (s *Server) pauseRoot(root rootRef, one ralphloop.OneIteration, dir string, e tickets.Epic, d mergeDecision) {
	project := root.Project
	reason, kind := d.result.Message, d.result.PauseKind
	s.events.publishEvent(Event{Type: EventRootPaused, Address: root.String(), Kind: kind, Level: EventLevelWarning, Message: reason})
	s.log.Warn("root paused", "root", root, "kind", kind, "reason", reason)
	if !d.sendNotice() {
		return
	}
	if s.chat.Enabled() {
		s.chat.Pause(project, s.chatOverride(project), one.Epic, one.Ticket.Path, one.Ticket.Identifier, string(schema.StatusLegacyAnswer), events.Kind(kind), reason, ralphloop.CountsOf(e))
	}
	// Only a needs-rebase pause is the epic's outcome; any other pause waits on
	// a person's fix and then merges.
	if kind == string(events.NeedsRebase) {
		s.notifyEpicOutcome(project, dir, one, e, false)
	}
}

// notifyEpicOutcome sends the epic-level notice. A one-off skips it.
func (s *Server) notifyEpicOutcome(project, dir string, one ralphloop.OneIteration, e tickets.Epic, merged bool) {
	// The epic, not one: a one-off's merge ticket is not its one-off file.
	if !s.chat.Enabled() || IsOneOffEpic(e) {
		return
	}
	elapsed, cost := ralphloop.EpicTotals(e)
	counts := ralphloop.CountsOf(e)
	counts.Recovered = recoveredCount(dir, one.Epic)
	s.chat.EpicComplete(project, s.chatOverride(project), one.Epic, counts, elapsed, cost, merged)
}

// submitRetro files the evolve one-off for a completed loop epic. It runs at
// the epic's final outcome, where completed_at is stamped: the merge, a root
// pause, or all-done with auto-ff-merge off. A refused submit is logged, never an error:
// the dedupe key refuses a second live retro of the same epic.
func (s *Server) submitRetro(root rootRef, projectDir string, e tickets.Epic) {
	if e.IsMap || IsOneOffEpic(e) || !e.EvolvedAt.IsZero() {
		return
	}
	pf, err := config.ReadProjectFile(projectDir)
	if err != nil {
		s.log.Warn("retro hook: read project file", "root", root, "err", err)
		return
	}
	mode := config.Config{Evolve: s.settings.Load().Evolve}.EvolveMode(pf)
	if mode == config.EvolveOff || mode == "" {
		return
	}
	if err := EnsureOutboxProject(s.cfg.TicketStore, mode); err != nil {
		s.log.Warn("retro hook: create outbox project", "root", root, "err", err)
		return
	}
	// A maintainer's retro writes to the gx project; without one it has nowhere to go.
	if mode == config.EvolveMaintainer {
		if _, err := s.projectDir("gx"); err != nil {
			s.log.Warn("retro hook: maintainer mode needs a gx project, not submitting", "root", root, "err", err)
			return
		}
	}
	res, err := s.oneOff(RetroSubmit(root.Project, filepath.Base(e.Path), time.Now()))
	switch {
	case err != nil:
		s.log.Warn("retro hook: submit", "root", root, "err", err)
	case res.Refused:
		s.log.Info("retro hook: submit refused", "root", root, "reason", res.Reason, "message", res.Message)
	}
}

// stampEpic stamps each of keys with now on the epic. A failed stamp is
// logged, never fatal: the merge it records has already happened.
func (s *Server) stampEpic(root rootRef, epicPath string, keys ...string) {
	now := time.Now().UTC()
	fields := make(map[string]any, len(keys))
	for _, k := range keys {
		fields[k] = now
	}
	if err := tickets.StampEpicFields(epicPath, fields); err != nil {
		s.log.Warn("stamp epic", "root", root, "keys", keys, "err", err)
	}
	// Every merge path stamps merged_at here, so an <map>-impl merge completes its map here.
	if slices.Contains(keys, "merged_at") {
		if _, err := tickets.MarkMapDone(epicPath, false); err != nil {
			s.log.Warn("mark map done", "root", root, "err", err)
		}
	}
}

// stampTotals records the epic's summed tokens, cost, Codex marker and friction.
// A failed stamp is logged, never fatal, like stampEpic's.
func (s *Server) stampTotals(root rootRef, e tickets.Epic) {
	if err := tickets.StampEpicFields(e.Path, tickets.SumEpicStats(e.Tickets).Fields()); err != nil {
		s.log.Warn("stamp epic totals", "root", root, "err", err)
	}
}

// stampMergedEpic records a merged epic's line counts and totals. A failed
// count (countErr) skips the line stamps; the totals are stamped regardless.
func (s *Server) stampMergedEpic(root rootRef, e tickets.Epic, added, removed int, countErr error) {
	if countErr != nil {
		s.log.Warn("count merged lines", "root", root, "err", countErr)
	} else {
		s.stampLines(root, e.Path, added, removed)
	}
	s.stampTotals(root, e)
}

// stampLines records the lines the merge added to and removed from trunk. A
// failed stamp is logged, never fatal, like stampEpic's.
func (s *Server) stampLines(root rootRef, epicPath string, added, removed int) {
	err := tickets.StampEpicFields(epicPath, map[string]any{"lines_added": added, "lines_removed": removed})
	if err != nil {
		s.log.Warn("stamp epic lines", "root", root, "err", err)
	}
}

const (
	mergeBlockKindKey   = "land_block_kind"
	mergeBlockReasonKey = "land_block_reason"
	mergeBlockFilesKey  = "land_block_files"
)

// mergePause is why a root's land paused. files is the full clashing list of a
// dirty-trunk pause; the reason text caps it.
type mergePause struct {
	reason string
	kind   events.Kind
	files  []string
}

func (s *Server) clearMergeBlock(root rootRef, epicPath string) {
	if err := tickets.ClearEpicFields(epicPath, mergeBlockKindKey, mergeBlockReasonKey, mergeBlockFilesKey); err != nil {
		s.log.Warn("clear merge block", "root", root, "err", err)
	}
}

// storeMergeBlock records a pause's kind, reason and files on the epic, so the
// epic's state outlives the event and a server restart.
func (s *Server) storeMergeBlock(root rootRef, epicPath string, p mergePause) {
	fields := map[string]any{mergeBlockReasonKey: p.reason, mergeBlockKindKey: string(p.kind)}
	if len(p.files) > 0 {
		fields[mergeBlockFilesKey] = p.files
	}
	if err := tickets.StampEpicFields(epicPath, fields); err != nil {
		s.log.Warn("store merge block", "root", root, "err", err)
	}
}

// nothingToMerge reports whether the root's feature branch is missing or has no
// commits ahead of its target. An error reading either means there may be
// something to merge: never call an epic merged on a guess.
func (s *Server) nothingToMerge(project string, epics []tickets.Epic, one ralphloop.OneIteration, repo string) bool {
	branch := "refs/heads/" + one.Epic
	if _, err := git.RevParse(repo, branch); err != nil {
		return errors.Is(err, git.ErrRefNotFound)
	}
	_, _, target, err := s.epicRefs(project, epics, one.Epic, repo)
	if err != nil {
		return false
	}
	merged, err := git.IsAncestor(repo, branch, target)
	return err == nil && merged
}

// mergeRoot fast-forwards the root's target to its feature branch through the
// merge core. A non-empty reason means the root must pause, not merge; kind
// names the pause's cause. Every error pauses the
// root: one left all-done in the queue looks stuck.
func (s *Server) mergeRoot(project string, epics []tickets.Epic, one ralphloop.OneIteration, repoDir string) mergePause {
	_, _, target, err := s.epicRefs(project, epics, one.Epic, repoDir)
	if err != nil {
		return s.refsFailed(project, one.Epic, err)
	}
	info, err := git.IdentifyDir(repoDir)
	if err != nil {
		return mergeFailed(project, one.Epic, err)
	}
	worktrees, err := git.ListWorktrees(info.Repo)
	if err != nil {
		return mergeFailed(project, one.Epic, err)
	}
	needsRebase := fmt.Sprintf("needs rebase: %s onto %s; rebase it, then merge again", one.Epic, target)
	out, err := s.mergeInto(info.Repo, worktrees, one.Epic, target)
	if err != nil {
		// Trunk moved outside the server between git's ff check and the ff:
		// that is a non-ff, not a failed merge.
		if ancestor, aerr := git.IsAncestor(info.Repo.Root, "refs/heads/"+target, one.Epic); aerr == nil && !ancestor {
			return mergePause{reason: needsRebase, kind: events.NeedsRebase}
		}
		// No pre-check: git decides whether local changes block the merge, and
		// only a refused merge looks at them.
		if dir := git.CheckoutDirForBranch(info.Repo, worktrees, target); dir != "" {
			if clashing := clashingFiles(info.Repo.Root, dir, target, one.Epic); len(clashing) > 0 {
				landed, lerr := s.landOverDirtyTrunk(project, info.Repo, dir, one.Epic, target, clashing)
				if lerr != nil {
					return mergeFailed(project, one.Epic, lerr)
				}
				if landed {
					return mergePause{}
				}
				return mergePause{
					reason: fmt.Sprintf("uncommitted changes on %s (%s) clash with %s; commit or discard them, then merge again",
						target, dir, listFiles(clashing)),
					kind:  events.DirtyTrunk,
					files: clashing,
				}
			}
		}
		return mergeFailed(project, one.Epic, err)
	}
	if out.Merged {
		return mergePause{}
	}
	return mergePause{reason: needsRebase, kind: events.NeedsRebase}
}

// refsFailed is the pause for an epic whose refs can't be worked out.
func (s *Server) refsFailed(project, epic string, err error) mergePause {
	if missing := (*tickets.ErrTrunkMissing)(nil); errors.As(err, &missing) {
		// Nothing merges; the project stops claiming until a person fixes trunk.
		s.enterTrunkMissing(project, err.Error())
		return mergePause{reason: fmt.Sprintf("the trunk branch is missing: %v; fix the trunk, then merge again", err), kind: events.TrunkMissing}
	}
	return mergeFailed(project, epic, err)
}

func mergeFailed(project, epic string, err error) mergePause {
	return mergePause{reason: fmt.Sprintf("merge failed: %v; fix it, then merge again", err), kind: events.MergeError}
}

// clashingFiles lists the uncommitted files in the target's checkout that the
// merge would also change: files that differ between the target's tip and the
// branch's. An unrelated local change doesn't block a merge, so it isn't listed.
func clashingFiles(repoRoot, dir, target, branch string) []string {
	dirty, err := git.UncommittedPaths(dir)
	if err != nil || len(dirty) == 0 {
		return nil
	}
	merged, err := git.DiffNames(repoRoot, "refs/heads/"+target, "refs/heads/"+branch)
	if err != nil {
		return nil
	}
	touched := make(map[string]bool, len(merged))
	for _, p := range merged {
		touched[p] = true
	}
	var clashing []string
	for _, p := range dirty {
		if touched[p] {
			clashing = append(clashing, p)
		}
	}
	return clashing
}

// maxReasonFiles caps the files a dirty-trunk reason lists.
const maxReasonFiles = 5

// listFiles lists the first maxReasonFiles paths, then "+N more".
func listFiles(paths []string) string {
	list := strings.Join(paths[:min(len(paths), maxReasonFiles)], ", ")
	if extra := len(paths) - maxReasonFiles; extra > 0 {
		list += fmt.Sprintf(" +%d more", extra)
	}
	return list
}
