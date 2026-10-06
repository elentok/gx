package repair

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// LandResult is the --json success payload of `gx tickets land`.
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
	if !in.IgnoreLiveTab && ralphloop.IterationTabLive(d, epic, t.Identifier) {
		return LandResult{}, "", &RefusalError{Reason: ReasonLiveAgentOnTab, Message: fmt.Sprintf("the herdr tab for ticket %s exists; close it or pass --ignore-live-tab", in.ID)}
	}

	wtDir, err := d.WorktreeDir(in.Cwd)
	if err != nil {
		return LandResult{}, "", err
	}
	featurePath := filepath.Join(wtDir, epic)
	rng, err := landSourceRange(in, d, featurePath, epic, t.Identifier)
	if err != nil {
		return LandResult{}, "", err
	}

	lockDir := filepath.Clean(in.EpicPath)
	if err := preflightLand(lockDir, epic, in.ID, featurePath, d); err != nil {
		return LandResult{}, "", err
	}
	if err := ralphloop.AcquireLandLockFor(lockDir, epic, in.ID); err != nil {
		if errors.Is(err, ralphloop.ErrLandLocked) {
			return LandResult{}, "", landLockedRefusal(lockDir, epic, in.ID)
		}
		return LandResult{}, "", err
	}
	conflicted := false
	defer func() {
		// A conflict keeps the lock: it must outlive this process until a
		// --continue or --abort clears it.
		if !conflicted {
			_ = ralphloop.ReleaseLandLock(lockDir)
		}
	}()

	prePickHead, err := d.RevParse(featurePath, "HEAD")
	if err != nil {
		return LandResult{}, "", fmt.Errorf("resolving %s HEAD: %w", epic, err)
	}
	session, recorded := ralphloop.RecoverLandSession(filepath.Dir(lockDir), epic, t.Identifier)
	res, err := ralphloop.LandTicket(ralphloop.LandDepsOf(d), ralphloop.LandParams{
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
		marker := ralphloop.LandMarker{Epic: epic, Ticket: in.ID, SourceRange: rng.Base + ".." + rng.Tip, PrePickHead: prePickHead}
		if err := ralphloop.WriteLandMarker(lockDir, marker); err != nil {
			return LandResult{}, "", fmt.Errorf("writing land marker: %w", err)
		}
		return out, fmt.Sprintf("%s: conflict landing %s; resolve it in %s, then --continue or --abort", EpicTicketLabel(in.EpicPath, in.ID), marker.SourceRange, featurePath), nil
	}

	if err := recordLanded(lockDir, epic, t, parsed.Status, res, session); err != nil {
		return LandResult{}, "", err
	}
	return out, fmt.Sprintf("%s: %s (%s)", EpicTicketLabel(in.EpicPath, in.ID), jsonOutcome(res.Outcome), res.SHA), nil
}

// landLockedRefusal names the lock's owner and, when no marker explains the
// lock, how to clear it.
func landLockedRefusal(lockDir, epic, id string) *RefusalError {
	msg := "another land is in progress (land lock is held)"
	owner, err := ralphloop.OrphanLandLock(lockDir)
	if err == nil && owner != nil {
		msg = fmt.Sprintf("land lock is held by %s with no conflict pending; if that land crashed, clear it with `gx tickets land %s %s --abort`", owner.Describe(), epic, id)
	}
	return &RefusalError{Reason: ReasonLandLocked, Message: msg}
}

// abortOrphanLock clears a lock that has no marker. It refuses while the
// owner is still running, since that is a live land, not a crash leftover.
func abortOrphanLock(lockDir, id string) (LandResult, string, error) {
	owner, err := ralphloop.ReadLandLock(lockDir)
	if err != nil {
		return LandResult{}, "", err
	}
	if owner == nil {
		return LandResult{}, "", &RefusalError{Reason: ReasonNoPendingLand, Message: fmt.Sprintf("no conflict landing ticket %s is pending", id)}
	}
	if owner.Alive() {
		return LandResult{}, "", &RefusalError{Reason: ReasonLandLocked, Message: fmt.Sprintf("land lock is held by running %s; wait for it to finish", owner.Describe())}
	}
	if err := ralphloop.ReleaseLandLock(lockDir); err != nil {
		return LandResult{}, "", fmt.Errorf("clearing land lock: %w", err)
	}
	return LandResult{Outcome: "unlocked"}, fmt.Sprintf("%s: cleared stale land lock held by %s", EpicTicketLabel(lockDir, id), owner.Describe()), nil
}

// recordLanded writes status: done (unless already) and the manual-land event
// through the park path.
func recordLanded(lockDir, epic string, t tickets.Ticket, status schema.Status, res ralphloop.LandResult, session ralphloop.LandSession) error {
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
	if err := ralphloop.RecordManualLand(filepath.Dir(lockDir), epic, t.Identifier, t.Path, status == schema.StatusDone, ev); err != nil {
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
	lockDir := filepath.Clean(in.EpicPath)
	marker, err := ralphloop.ReadLandMarker(lockDir)
	if err != nil {
		return LandResult{}, "", err
	}
	if marker == nil && in.Abort {
		return abortOrphanLock(lockDir, in.ID)
	}
	if marker == nil || marker.Ticket != in.ID {
		return LandResult{}, "", &RefusalError{Reason: ReasonNoPendingLand, Message: fmt.Sprintf("no conflict landing ticket %s is pending", in.ID)}
	}
	wtDir, err := d.WorktreeDir(in.Cwd)
	if err != nil {
		return LandResult{}, "", err
	}
	featurePath := filepath.Join(wtDir, epic)
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
		if err := ralphloop.ClearLand(lockDir); err != nil {
			return LandResult{}, "", fmt.Errorf("clearing land marker: %w", err)
		}
		return LandResult{Outcome: "aborted"}, fmt.Sprintf("%s: land aborted", EpicTicketLabel(in.EpicPath, in.ID)), nil
	}

	if inProgress {
		return LandResult{}, "", &RefusalError{Reason: ReasonLandConflictPending, Message: fmt.Sprintf("the cherry-pick for ticket %s is still in progress; resolve it and run `git cherry-pick --continue` first", in.ID)}
	}
	head, err := d.RevParse(featurePath, "HEAD")
	if err != nil {
		return LandResult{}, "", fmt.Errorf("resolving %s HEAD: %w", epic, err)
	}
	if head == marker.PrePickHead {
		return LandResult{}, "", &RefusalError{Reason: ReasonLandNotResolved, Message: fmt.Sprintf("%s has not moved since the conflict; nothing was committed (use --abort to give up)", epic)}
	}

	parsed, err := schema.ParseTicket(t.Path)
	if err != nil {
		return LandResult{}, "", err
	}
	session, _ := ralphloop.RecoverLandSession(filepath.Dir(lockDir), epic, t.Identifier)
	res, err := ralphloop.StampLanded(ralphloop.LandDepsOf(d), ralphloop.LandParams{
		FeatureWorktree: featurePath,
		FeatureBranch:   epic,
		TicketID:        t.Identifier,
		TicketPath:      t.Path,
		Session:         session,
	})
	if err != nil {
		return LandResult{}, "", err
	}
	if err := recordLanded(lockDir, epic, t, parsed.Status, res, session); err != nil {
		return LandResult{}, "", err
	}
	if err := ralphloop.ClearLand(lockDir); err != nil {
		return LandResult{}, "", fmt.Errorf("clearing land marker: %w", err)
	}
	out := LandResult{Outcome: jsonOutcome(res.Outcome), SHA: res.SHA, TrailerValue: res.TrailerValue, MetricsStamped: res.MetricsStamped}
	return out, fmt.Sprintf("%s: %s (%s)", EpicTicketLabel(in.EpicPath, in.ID), out.Outcome, res.SHA), nil
}

// checkLandable applies the status and commitless rules. Commitless is the
// explicit flag only, never the type-derived notion.
func checkLandable(id string, t schema.Ticket) error {
	switch t.Status {
	case schema.StatusDone, schema.StatusClaimed, schema.StatusNeedsRepair:
	default:
		return &RefusalError{Reason: ReasonStatusRefused, Message: fmt.Sprintf("ticket %s is %s; land accepts only done, claimed or needs-repair", id, t.Status)}
	}
	if t.Commitless {
		return &RefusalError{Reason: ReasonCommitless, Message: fmt.Sprintf("ticket %s is flagged commitless; there is nothing to land", id)}
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
	verdict, err := ralphloop.LandPreflight(lockDir, epic, id, inProgress)
	if err != nil {
		return err
	}
	switch verdict {
	case ralphloop.LandConflictPending:
		return &RefusalError{Reason: ReasonLandConflictPending, Message: fmt.Sprintf("a conflict landing ticket %s is pending; resolve it, then --continue or --abort", id)}
	case ralphloop.LandOtherTicket:
		return &RefusalError{Reason: ReasonLandBlocked, Message: "a conflict landing a different ticket is pending; finish or abort it first"}
	case ralphloop.LandUnknownPick:
		return &RefusalError{Reason: ReasonLandBlocked, Message: "a cherry-pick is in progress on the feature branch with no land marker; refusing to trample it"}
	}
	return nil
}

// jsonOutcome maps the Go outcome to the spec's kebab-case JSON vocabulary.
func jsonOutcome(o ralphloop.LandOutcome) string {
	if o == ralphloop.AlreadyApplied {
		return "already-applied"
	}
	return string(o)
}
