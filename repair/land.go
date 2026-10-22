package repair

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// LandResult is the --json success payload of `gx server tickets merge`.
type LandResult struct {
	Outcome        string `json:"outcome"`
	SHA            string `json:"sha"`
	TrailerValue   string `json:"trailer_value"`
	MetricsStamped bool   `json:"metrics_stamped"`
}

// LandInput is everything Land needs from its caller.
type LandInput struct {
	EpicPath      string
	ID            string
	From, To      string
	IgnoreLiveTab bool
	Continue      bool
	Abort         bool
	Cwd           string
	Getwd         func() (string, error)
}

// Land lands a stuck ticket's commits (or continues/aborts a conflicted land)
// and returns the result with a human-readable summary line.
func Land(in LandInput, d ralphloop.Deps) (LandResult, string, error) {
	if branch, ok := IsRalphLoopBranch(in.Getwd); ok {
		return LandResult{}, "", &RefusalError{Reason: ReasonRalphLoopCwd, Message: fmt.Sprintf("refusing to land from a ralph-loop/* working directory (%s); run from outside the iteration worktree", branch)}
	}
	if in.Continue && in.Abort {
		return LandResult{}, "", errors.New("--continue and --abort are mutually exclusive")
	}
	if in.Continue || in.Abort {
		return resolveLand(in, d)
	}
	if (in.From == "") != (in.To == "") {
		return LandResult{}, "", errors.New("--from and --to must be given together")
	}

	_, t, err := FindEpicTicket(in.EpicPath, in.ID)
	if err != nil {
		return LandResult{}, "", err
	}
	parsed, err := schema.ParseTicket(t.Path)
	if err != nil {
		return LandResult{}, "", err
	}
	if err := checkLandable(in.ID, parsed); err != nil {
		return LandResult{}, "", err
	}

	epic := filepath.Base(filepath.Clean(in.EpicPath))
	if _, live := ralphloop.FindIterationSession(d, epic, t.Identifier); live && !in.IgnoreLiveTab {
		return LandResult{}, "", &RefusalError{Reason: ReasonLiveAgentOnTab, Message: fmt.Sprintf("the agent session for ticket %s exists; stop it or pass --ignore-live-tab", in.ID)}
	}

	featurePath, err := featureWorktree(d, in.Cwd, epic)
	if err != nil {
		return LandResult{}, "", err
	}
	if parsed.PausedClass() == schema.PauseClassAnswer {
		if err := checkRecoverable(d, featurePath, epic, t.Identifier); err != nil {
			return LandResult{}, "", err
		}
	}
	rng, err := landSourceRange(in, d, featurePath, epic, t.Identifier)
	if err != nil {
		return LandResult{}, "", err
	}

	lockDir, err := ralphloop.MergeLockDir(in.EpicPath)
	if err != nil {
		return LandResult{}, "", err
	}
	if err := preflightLand(lockDir, epic, in.ID, featurePath, d); err != nil {
		return LandResult{}, "", err
	}
	if err := ralphloop.AcquireMergeLockFor(lockDir, epic, in.ID); err != nil {
		if errors.Is(err, ralphloop.ErrMergeLocked) {
			return LandResult{}, "", landLockedRefusal(lockDir, in.EpicPath, in.ID)
		}
		return LandResult{}, "", err
	}
	conflicted := false
	defer func() {
		// A conflict keeps the lock: it must outlive this process until a
		// --continue or --abort clears it.
		if !conflicted {
			_ = ralphloop.ReleaseMergeLock(lockDir)
		}
	}()

	prePickHead, err := d.RevParse(featurePath, "HEAD")
	if err != nil {
		return LandResult{}, "", fmt.Errorf("resolving %s HEAD: %w", epic, err)
	}
	session, recorded := ralphloop.RecoverMergeSession(filepath.Dir(filepath.Clean(in.EpicPath)), epic, t.Identifier)
	res, err := ralphloop.MergeTicket(ralphloop.MergeDepsOf(d), ralphloop.MergeParams{
		FeatureWorktree: featurePath,
		FeatureBranch:   epic,
		TicketID:        t.Identifier,
		TicketPath:      t.Path,
		SourceRange:     rng,
		RecordedSHA:     recorded,
		Session:         session,
	})
	if err != nil {
		return LandResult{}, "", err
	}

	out := LandResult{Outcome: jsonOutcome(res.Outcome), SHA: res.SHA, TrailerValue: res.TrailerValue, MetricsStamped: res.MetricsStamped}
	if res.Outcome == ralphloop.Conflicted {
		conflicted = true
		marker := ralphloop.MergeMarker{Epic: epic, Ticket: in.ID, SourceRange: rng.Base + ".." + rng.Tip, PrePickHead: prePickHead}
		if err := ralphloop.WriteMergeMarker(lockDir, marker); err != nil {
			return LandResult{}, "", fmt.Errorf("writing land marker: %w", err)
		}
		return out, fmt.Sprintf("%s: conflict landing %s; resolve it in %s, then --continue or --abort", EpicTicketLabel(in.EpicPath, in.ID), marker.SourceRange, featurePath), nil
	}

	if err := recordLanded(in.EpicPath, epic, t, parsed.Status, res, session); err != nil {
		return LandResult{}, "", err
	}
	return out, fmt.Sprintf("%s: %s (%s)", EpicTicketLabel(in.EpicPath, in.ID), jsonOutcome(res.Outcome), res.SHA), nil
}

// landLockedRefusal names the lock's owner and, when no marker explains the
// lock, how to clear it.
func landLockedRefusal(lockDir, epicPath, id string) *RefusalError {
	msg := "another land is in progress (land lock is held)"
	owner, err := ralphloop.OrphanMergeLock(lockDir)
	if err == nil && owner != nil {
		msg = fmt.Sprintf("land lock is held by %s with no conflict pending; if that land crashed, clear it with `gx server tickets merge %s --abort`", owner.Describe(), EpicTicketLabel(epicPath, id))
	}
	return &RefusalError{Reason: ReasonLandLocked, Message: msg}
}

// abortOrphanLock clears a lock that has no marker. It refuses while the
// owner is still running, since that is a live land, not a crash leftover.
func abortOrphanLock(epicPath, lockDir, id string) (LandResult, string, error) {
	owner, err := ralphloop.ReadMergeLock(lockDir)
	if err != nil {
		return LandResult{}, "", err
	}
	if owner == nil {
		return LandResult{}, "", &RefusalError{Reason: ReasonNoPendingMerge, Message: fmt.Sprintf("no conflicted merge of ticket %s is pending", id)}
	}
	if owner.Alive() {
		return LandResult{}, "", &RefusalError{Reason: ReasonLandLocked, Message: fmt.Sprintf("land lock is held by running %s; wait for it to finish", owner.Describe())}
	}
	if err := ralphloop.ReleaseMergeLock(lockDir); err != nil {
		return LandResult{}, "", fmt.Errorf("clearing land lock: %w", err)
	}
	return LandResult{Outcome: "unlocked"}, fmt.Sprintf("%s: cleared stale land lock held by %s", EpicTicketLabel(epicPath, id), owner.Describe()), nil
}

// recordLanded writes status: done (unless already) and the manual-merge event
// through the park path.
func recordLanded(epicPath, epic string, t tickets.Ticket, status schema.Status, res ralphloop.MergeResult, session ralphloop.MergeSession) error {
	ev := ralphloop.Event{
		Outcome:      string(res.Outcome),
		SHA:          res.SHA,
		TrailerValue: res.TrailerValue,
		AgentSession: session.ID,
		Agent:        session.Agent,
	}
	if session.ID == "" {
		ev.Reason = "no recoverable agent session; metrics not stamped"
	}
	if err := ralphloop.RecordManualMerge(filepath.Dir(filepath.Clean(epicPath)), epic, t.Identifier, t.Path, status == schema.StatusDone, ev); err != nil {
		return fmt.Errorf("marking ticket %s done: %w", t.Identifier, err)
	}
	return nil
}

// resolveLand finishes (--continue) or abandons (--abort) the conflict a
// previous land left pending for in.ID.
func resolveLand(in LandInput, d ralphloop.Deps) (LandResult, string, error) {
	if in.From != "" || in.To != "" {
		return LandResult{}, "", errors.New("--from/--to cannot be combined with --continue or --abort")
	}
	_, t, err := FindEpicTicket(in.EpicPath, in.ID)
	if err != nil {
		return LandResult{}, "", err
	}
	epic := filepath.Base(filepath.Clean(in.EpicPath))
	lockDir, err := ralphloop.MergeLockDir(in.EpicPath)
	if err != nil {
		return LandResult{}, "", err
	}
	marker, err := ralphloop.ReadMergeMarker(lockDir)
	if err != nil {
		return LandResult{}, "", err
	}
	if marker == nil && in.Abort {
		return abortOrphanLock(in.EpicPath, lockDir, in.ID)
	}
	if marker == nil || marker.Ticket != in.ID {
		return LandResult{}, "", &RefusalError{Reason: ReasonNoPendingMerge, Message: fmt.Sprintf("no conflicted merge of ticket %s is pending", in.ID)}
	}
	featurePath, err := featureWorktree(d, in.Cwd, epic)
	if err != nil {
		return LandResult{}, "", err
	}
	inProgress, err := d.CherryPickInProgress(featurePath)
	if err != nil {
		return LandResult{}, "", fmt.Errorf("checking cherry-pick state: %w", err)
	}

	if in.Abort {
		if inProgress {
			if err := d.AbortCherryPick(featurePath); err != nil {
				return LandResult{}, "", fmt.Errorf("aborting cherry-pick: %w", err)
			}
		}
		if err := ralphloop.ClearMerge(lockDir); err != nil {
			return LandResult{}, "", fmt.Errorf("clearing land marker: %w", err)
		}
		return LandResult{Outcome: "aborted"}, fmt.Sprintf("%s: merge aborted", EpicTicketLabel(in.EpicPath, in.ID)), nil
	}

	if inProgress {
		return LandResult{}, "", &RefusalError{Reason: ReasonMergeConflictPending, Message: fmt.Sprintf("the cherry-pick for ticket %s is still in progress; resolve it and run `git cherry-pick --continue` first", in.ID)}
	}
	head, err := d.RevParse(featurePath, "HEAD")
	if err != nil {
		return LandResult{}, "", fmt.Errorf("resolving %s HEAD: %w", epic, err)
	}
	if head == marker.PrePickHead {
		return LandResult{}, "", &RefusalError{Reason: ReasonMergeNotResolved, Message: fmt.Sprintf("%s has not moved since the conflict; nothing was committed (use --abort to give up)", epic)}
	}
	// HEAD moving is not proof this ticket's pick finished: other landings move
	// it too, and stamping would then mark one of their commits as this one.
	// The resolved pick is unstamped, so any ticket trailer since the conflict
	// is someone else's landing.
	if ok, err := d.IsAncestor(featurePath, marker.PrePickHead, head); err != nil || !ok {
		return LandResult{}, "", &RefusalError{Reason: ReasonMergeSuperseded, Message: fmt.Sprintf("%s no longer descends from the pre-conflict head %s; the conflicted pick for ticket %s is gone (run --abort, then merge again)", epic, marker.PrePickHead, in.ID)}
	}
	if others, err := d.TrailerMap(featurePath, marker.PrePickHead+"..HEAD", ralphloop.TicketTrailerKey); err != nil {
		return LandResult{}, "", fmt.Errorf("reading landings since the conflict on %s: %w", epic, err)
	} else if len(others) > 0 {
		return LandResult{}, "", &RefusalError{Reason: ReasonMergeSuperseded, Message: fmt.Sprintf("other tickets merged into %s since the conflict; the conflicted pick for ticket %s is gone (run --abort, then merge again)", epic, in.ID)}
	}

	parsed, err := schema.ParseTicket(t.Path)
	if err != nil {
		return LandResult{}, "", err
	}
	session, _ := ralphloop.RecoverMergeSession(filepath.Dir(filepath.Clean(in.EpicPath)), epic, t.Identifier)
	res, err := ralphloop.StampMerged(ralphloop.MergeDepsOf(d), ralphloop.MergeParams{
		FeatureWorktree: featurePath,
		FeatureBranch:   epic,
		TicketID:        t.Identifier,
		TicketPath:      t.Path,
		Session:         session,
	})
	if err != nil {
		return LandResult{}, "", err
	}
	if err := recordLanded(in.EpicPath, epic, t, parsed.Status, res, session); err != nil {
		return LandResult{}, "", err
	}
	if err := ralphloop.ClearMerge(lockDir); err != nil {
		return LandResult{}, "", fmt.Errorf("clearing land marker: %w", err)
	}
	out := LandResult{Outcome: jsonOutcome(res.Outcome), SHA: res.SHA, TrailerValue: res.TrailerValue, MetricsStamped: res.MetricsStamped}
	return out, fmt.Sprintf("%s: %s (%s)", EpicTicketLabel(in.EpicPath, in.ID), out.Outcome, res.SHA), nil
}

// featureWorktree is the epic's feature worktree in cwd's repo. Without one
// the caller is in the wrong repo, which must not read as a missing branch.
func featureWorktree(d ralphloop.Deps, cwd, epic string) (string, error) {
	if wtDir, err := d.WorktreeDir(cwd); err == nil {
		featurePath := filepath.Join(wtDir, epic)
		if _, err := d.RevParse(featurePath, "HEAD"); err == nil {
			return featurePath, nil
		}
	}
	return "", &RefusalError{Reason: ReasonWrongRepo, Message: fmt.Sprintf("wrong repo, run from the repo holding epic %s (%s has no %s worktree)", epic, cwd, epic)}
}

// checkLandable applies the status and commitless rules. Commitless is the
// explicit flag only, never the type-derived notion. A needs-answer ticket
// lands only as a zero-commit park, a late commit that came with no finish.
func checkLandable(id string, t schema.Ticket) error {
	switch {
	case t.Status == schema.StatusDone, t.Status == schema.StatusClaimed, t.PausedClass() == schema.PauseClassRepair:
	case t.PausedClass() == schema.PauseClassAnswer && (t.PauseReason == "" || t.PauseReason == schema.PauseZeroCommit):
	default:
		return &RefusalError{Reason: ReasonStatusRefused, Message: fmt.Sprintf("ticket %s is %s; merge accepts only done, claimed, needs-repair or a zero-commit needs-answer", id, describeStatus(t))}
	}
	if t.Commitless {
		return &RefusalError{Reason: ReasonCommitless, Message: fmt.Sprintf("ticket %s is flagged commitless; there is nothing to merge", id)}
	}
	return nil
}

func describeStatus(t schema.Ticket) string {
	if class := t.PausedClass(); class != "" {
		if class == schema.PauseClassAnswer && t.PauseReason != "" {
			return fmt.Sprintf("%s (%s)", class.LegacyStatus(), t.PauseReason.LegacyKind())
		}
		return string(class.LegacyStatus())
	}
	return string(t.Status)
}

// checkRecoverable is verify's recoverable test: the iteration branch exists
// and has commits past its merge-base with the feature branch.
func checkRecoverable(d ralphloop.Deps, featurePath, epic, identifier string) error {
	branch := ralphloop.IterationBranch(epic, identifier)
	if _, err := d.RevParse(featurePath, branch); err != nil {
		return &RefusalError{Reason: ReasonIterationBranchMissing, Message: fmt.Sprintf("no iteration branch %s; commits not recoverable", branch)}
	}
	base, err := d.MergeBase(featurePath, branch, epic)
	if err != nil {
		return fmt.Errorf("resolving merge-base of %s and %s: %w", branch, epic, err)
	}
	ahead, err := d.CommitsAhead(featurePath, base, branch)
	if err != nil {
		return fmt.Errorf("counting commits on %s: %w", branch, err)
	}
	if ahead == 0 {
		return &RefusalError{Reason: ReasonNotRecoverable, Message: fmt.Sprintf("%s has no commits past %s; nothing to land", branch, epic)}
	}
	return nil
}

// landSourceRange is --from/--to when given, else the iteration branch's
// commits since its merge-base with the feature branch.
func landSourceRange(in LandInput, d ralphloop.Deps, featurePath, epic, identifier string) (ralphloop.SourceRange, error) {
	if in.From != "" {
		return ralphloop.SourceRange{Base: in.From, Tip: in.To}, nil
	}
	branch := ralphloop.IterationBranch(epic, identifier)
	if _, err := d.RevParse(featurePath, branch); err != nil {
		return ralphloop.SourceRange{}, &RefusalError{Reason: ReasonIterationBranchMissing, Message: fmt.Sprintf("no iteration branch %s; commits not recoverable (--from/--to can still land an explicit range)", branch)}
	}
	base, err := d.MergeBase(featurePath, branch, epic)
	if err != nil {
		return ralphloop.SourceRange{}, fmt.Errorf("resolving merge-base of %s and %s: %w", branch, epic, err)
	}
	return ralphloop.SourceRange{Base: base, Tip: branch}, nil
}

// preflightLand is the three-way marker/cherry-pick check.
func preflightLand(lockDir, epic, id, featurePath string, d ralphloop.Deps) error {
	inProgress, err := d.CherryPickInProgress(featurePath)
	if err != nil {
		return fmt.Errorf("checking cherry-pick state: %w", err)
	}
	verdict, err := ralphloop.MergePreflight(lockDir, epic, id, inProgress)
	if err != nil {
		return err
	}
	switch verdict {
	case ralphloop.MergeConflictPending:
		return &RefusalError{Reason: ReasonMergeConflictPending, Message: fmt.Sprintf("a conflicted merge of ticket %s is pending; resolve it, then --continue or --abort", id)}
	case ralphloop.MergeOtherTicket:
		return &RefusalError{Reason: ReasonMergeBlocked, Message: "a conflicted merge of a different ticket is pending; finish or abort it first"}
	case ralphloop.MergeUnknownPick:
		return &RefusalError{Reason: ReasonMergeBlocked, Message: "a cherry-pick is in progress on the feature branch with no merge marker; refusing to trample it"}
	}
	return nil
}

// jsonOutcome maps the Go outcome to the spec's kebab-case JSON vocabulary.
func jsonOutcome(o ralphloop.MergeOutcome) string {
	if o == ralphloop.AlreadyApplied {
		return "already-applied"
	}
	return string(o)
}
