package server

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// completeRootIfDone lands the root's feature branch once every ticket in its
// epic is done, then dequeues it. A branch that needs a rebase gets a merge
// ticket instead, or parks the root at the cap. With the project's auto-ff-merge off,
// and no manual merge ticket in the epic, the
// branch is left alone: the root is dequeued as complete and a person runs gx-merge.
// completed_at marks the epic's final outcome: with auto-ff-merge on that is
// the land or the park, not all-done. An epic with nothing to land is merged
// as it stands, whatever auto-ff-merge says.
func (s *Server) completeRootIfDone(root rootRef, one ralphloop.OneIteration) {
	// The one-offs epic is a standing inbox: it never lands, merges or retros.
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

// completeEpic is completeRootIfDone for the all-done epic e. The land decides
// under its trunk's lock, so a second land into that trunk sees the first one's
// result; notices and the retro go after unlock, so claiming elsewhere never
// waits on them.
//
// Without refs there is no target, so the land locks the epic itself: landEpic
// parks it the way landRoot does, and two finishes still can't both stamp.
func (s *Server) completeEpic(root rootRef, one ralphloop.OneIteration, dir, repo string, epics []tickets.Epic, e tickets.Epic) {
	_, _, target, err := s.epicRefs(root.Project, epics, one.Epic, repo)
	lockKey := target
	if err != nil {
		lockKey = "epic:" + root.String()
	}
	unlock := s.trunkLocks.lock(repo, lockKey)
	d := s.landEpic(root, one, repo, epics, e, target, false)
	unlock()
	s.actOnLand(root, one, dir, e, d)
}

// landOutcome is how a land ended; the zero value is a refusal with nothing
// to act on.
type landOutcome int

const (
	outcomeNone landOutcome = iota
	// outcomeMerged: the epic landed (or had nothing to land) and was dequeued.
	outcomeMerged
	// outcomeCompleted: all-done with auto-ff-merge off; the branch is left alone.
	outcomeCompleted
	outcomeParked
	// outcomeMergeTicket: a non-ff forked a merge ticket; the root stays queued.
	outcomeMergeTicket
	// outcomeAlreadyMerged: merged elsewhere and already dequeued.
	outcomeAlreadyMerged
)

// landDecision is what landEpic decided under the lock, for its caller to act
// on after unlock. first is set only for the caller that made the
// completed_at or merged_at stamp transition: that keeps the retro and the
// epic notice once-only without holding the lock around them.
type landDecision struct {
	result  QueueResult
	outcome landOutcome
	first   bool
}

// completed reports whether the root left the queue as a final outcome.
func (d landDecision) completed() bool {
	return d.outcome == outcomeMerged || d.outcome == outcomeCompleted
}

// sendNotice: a merge always speaks, as an epic merged elsewhere notifies with
// its dequeue. A park with a kind waits on a person's fix, so its chat notice
// goes every time; a needs-rebase park and a completion are the epic's outcome
// and speak only when their stamp is new.
func (d landDecision) sendNotice() bool {
	switch d.outcome {
	case outcomeMerged:
		return true
	case outcomeCompleted:
		return d.first
	case outcomeParked:
		return d.result.ParkKind != "" || d.first
	}
	return false
}

func (d landDecision) submitRetro() bool {
	switch d.outcome {
	case outcomeMerged, outcomeCompleted, outcomeParked, outcomeAlreadyMerged:
		return d.first
	}
	return false
}

func parkDecision(first bool, reason string, kind events.Kind) landDecision {
	return landDecision{
		result:  QueueResult{Parked: true, ParkKind: string(kind), Message: reason},
		outcome: outcomeParked,
		first:   first,
	}
}

// landEpic is completeEpic's work under target's trunk lock: stamps, git and
// the dequeue. force lands with auto-ff-merge off: the manual trigger's direct
// land.
func (s *Server) landEpic(root rootRef, one ralphloop.OneIteration, repo string, epics []tickets.Epic, e tickets.Epic, target string, force bool) landDecision {
	project := root.Project
	// A stored block is the last land's; this land decides afresh, and a new
	// park writes it again.
	if err := tickets.ClearEpicFields(e.Path, landBlockKindKey, landBlockReasonKey); err != nil {
		s.log.Warn("clear land block", "root", root, "err", err)
	}
	fresh := tickets.LoadEpicTicketMD(e.Path)
	already := !fresh.MergedAt.IsZero()
	first := fresh.CompletedAt.IsZero()
	merged := true
	switch {
	case already:
		if first {
			s.stampEpic(root, e.Path, "completed_at")
		}
	case s.nothingToLand(project, epics, one, repo):
		s.stampEpic(root, e.Path, "merged_at", "completed_at")
		first = true
	case force || s.autoFFMerge(project) || hasManualMergeTicket(e):
		reason, kind := s.landRoot(project, epics, one, repo)
		// A non-ff below the cap is not an outcome: the merge ticket makes the
		// epic not all-done, so the root stays queued and its finish lands again.
		// The ticket is never manual, even under force: it counts against the cap.
		if reason != "" && kind == "" {
			if id := s.forkMergeTicket(root, e, target, false); id != "" {
				return landDecision{result: QueueResult{MergeTicket: tickets.Address{Project: project, Epic: root.Epic, ID: id}.String()}, outcome: outcomeMergeTicket}
			}
		}
		if reason != "" {
			s.stampEpic(root, e.Path, "completed_at")
			s.storeLandBlock(root, e.Path, kind, reason)
			return parkDecision(first, reason, kind)
		}
		s.stampEpic(root, e.Path, "merged_at", "completed_at")
		first = true
	default:
		s.stampEpic(root, e.Path, "completed_at")
		merged = false
	}
	d := landDecision{result: QueueResult{Merged: merged}, outcome: outcomeCompleted, first: first}
	if merged {
		d.outcome = outcomeMerged
	}
	removed, err := s.queued.removeRoot(root)
	if err != nil {
		s.log.Warn("dequeue completed root", "root", root, "err", err)
	} else if removed {
		s.events.publish(EventQueueChanged, root.String())
	}
	// An epic merged elsewhere notifies with the dequeue, so a double land sends one.
	if already && !removed {
		d.outcome = outcomeAlreadyMerged
	}
	return d
}

// actOnLand sends d's notices and retro. It runs after the trunk lock is
// released, and returns d's result.
func (s *Server) actOnLand(root rootRef, one ralphloop.OneIteration, dir string, e tickets.Epic, d landDecision) QueueResult {
	if d.result.Parked {
		s.parkRoot(root, one, dir, e, d)
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

// parkRoot reports the park of e's all-done root; an empty kind is a
// needs-rebase park.
func (s *Server) parkRoot(root rootRef, one ralphloop.OneIteration, dir string, e tickets.Epic, d landDecision) {
	project := root.Project
	reason, kind := d.result.Message, d.result.ParkKind
	s.events.publishEvent(Event{Type: EventRootParked, Address: root.String(), Kind: kind, Level: EventLevelWarning, Message: reason})
	s.log.Warn("root parked", "root", root, "kind", kind, "reason", reason)
	if !d.sendNotice() {
		return
	}
	if s.chat.Enabled() {
		s.chat.Park(project, s.chatOverride(project), one.Epic, one.Ticket.Path, one.Ticket.Identifier, string(schema.StatusNeedsAnswer), events.Kind(kind), reason, ralphloop.CountsOf(e))
	}
	// Only a needs-rebase park, the one without a kind, is the
	// epic's outcome; any other park waits on a person's fix and then lands.
	if kind == "" {
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
// the epic's final outcome, where completed_at is stamped: the land, a root
// park, or all-done with auto-ff-merge off. A refused submit is logged, never an error:
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
// logged, never fatal: the land it records has already happened.
func (s *Server) stampEpic(root rootRef, epicPath string, keys ...string) {
	now := time.Now().UTC()
	fields := make(map[string]any, len(keys))
	for _, k := range keys {
		fields[k] = now
	}
	if err := tickets.StampEpicFields(epicPath, fields); err != nil {
		s.log.Warn("stamp epic", "root", root, "keys", keys, "err", err)
	}
}

const (
	landBlockKindKey   = "land_block_kind"
	landBlockReasonKey = "land_block_reason"
)

// storeLandBlock records a park's kind and reason on the epic, so the epic's
// state outlives the event and a server restart. A needs-rebase park has no kind.
func (s *Server) storeLandBlock(root rootRef, epicPath string, kind events.Kind, reason string) {
	fields := map[string]any{landBlockReasonKey: reason}
	if kind != "" {
		fields[landBlockKindKey] = string(kind)
	}
	if err := tickets.StampEpicFields(epicPath, fields); err != nil {
		s.log.Warn("store land block", "root", root, "err", err)
	}
}

// nothingToLand reports whether the root's feature branch is missing or has no
// commits ahead of its target. An error reading either means there may be
// something to land: never call an epic merged on a guess.
func (s *Server) nothingToLand(project string, epics []tickets.Epic, one ralphloop.OneIteration, repo string) bool {
	branch := "refs/heads/" + one.Epic
	if _, err := git.RevParse(repo, branch); err != nil {
		return errors.Is(err, git.ErrRefNotFound)
	}
	_, _, target, err := s.epicRefs(project, epics, one.Epic, repo)
	if err != nil {
		return false
	}
	landed, err := git.IsAncestor(repo, branch, target)
	return err == nil && landed
}

// landRoot fast-forwards the root's target to its feature branch through the
// merge core. A non-empty reason means the root must park, not land; kind
// names the park's cause, empty for a needs-rebase one. Every error parks the
// root: one left all-done in the queue looks stuck.
func (s *Server) landRoot(project string, epics []tickets.Epic, one ralphloop.OneIteration, repoDir string) (reason string, kind events.Kind) {
	_, _, target, err := s.epicRefs(project, epics, one.Epic, repoDir)
	if err != nil {
		return s.refsFailed(project, one.Epic, err)
	}
	info, err := git.IdentifyDir(repoDir)
	if err != nil {
		return landFailed(project, one.Epic, err)
	}
	worktrees, err := git.ListWorktrees(info.Repo)
	if err != nil {
		return landFailed(project, one.Epic, err)
	}
	needsRebase := fmt.Sprintf("needs rebase: %s onto %s; rebase it, then merge again", one.Epic, target)
	out, err := s.mergeInto(info.Repo, worktrees, one.Epic, target)
	if err != nil {
		// Trunk moved outside the server between git's ff check and the ff:
		// that is a non-ff, not a failed land.
		if ancestor, aerr := git.IsAncestor(info.Repo.Root, "refs/heads/"+target, one.Epic); aerr == nil && !ancestor {
			return needsRebase, ""
		}
		// No pre-check: git decides whether local changes block the land, and
		// only a refused land looks at them.
		if dir := git.CheckoutDirForBranch(info.Repo, worktrees, target); dir != "" {
			if clashing := clashingFiles(info.Repo.Root, dir, target, one.Epic); len(clashing) > 0 {
				return fmt.Sprintf("uncommitted changes on %s (%s) clash with %s; commit or discard them, then merge again",
					target, dir, listFiles(clashing)), events.DirtyTrunk
			}
		}
		return landFailed(project, one.Epic, err)
	}
	if out.Merged {
		return "", ""
	}
	return needsRebase, ""
}

// refsFailed is the park reason and kind for an epic whose refs can't be
// worked out.
func (s *Server) refsFailed(project, epic string, err error) (string, events.Kind) {
	if missing := (*tickets.ErrTrunkMissing)(nil); errors.As(err, &missing) {
		// Nothing lands; the project stops claiming until a person fixes trunk.
		s.enterTrunkMissing(project, err.Error())
		return fmt.Sprintf("the trunk branch is missing: %v; fix the trunk, then merge again", err), events.TrunkMissing
	}
	return landFailed(project, epic, err)
}

func landFailed(project, epic string, err error) (string, events.Kind) {
	return fmt.Sprintf("land failed: %v; fix it, then merge again", err), events.LandError
}

// clashingFiles lists the uncommitted files in the target's checkout that the
// land would also change: files that differ between the target's tip and the
// branch's. An unrelated local change doesn't block a land, so it isn't listed.
func clashingFiles(repoRoot, dir, target, branch string) []string {
	changes, err := git.UncommittedChanges(dir)
	if err != nil || len(changes) == 0 {
		return nil
	}
	landed, err := git.DiffNames(repoRoot, "refs/heads/"+target, "refs/heads/"+branch)
	if err != nil {
		return nil
	}
	touched := make(map[string]bool, len(landed))
	for _, p := range landed {
		touched[p] = true
	}
	var clashing []string
	for _, c := range changes {
		if touched[c.Path] {
			clashing = append(clashing, c.Path)
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
