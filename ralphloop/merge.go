package ralphloop

import (
	"fmt"
	"os"
	"strconv"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// MergeOutcome is what MergeTicket did with a ticket's commits.
type MergeOutcome string

const (
	// Merged: the source range was cherry-picked and stamped.
	Merged MergeOutcome = "merged"
	// AlreadyApplied: the evidence ladder found the commits on the feature
	// branch already; nothing was picked and no trailer was re-stamped.
	AlreadyApplied MergeOutcome = "already_applied"
	// Conflicted: the pick stopped on a conflict. Sequencer state is left
	// intact for the caller to resolve or abort; nothing was stamped.
	Conflicted MergeOutcome = "conflicted"
)

// SourceRange is the commit range base..Tip (base exclusive) a merging picks.
type SourceRange struct {
	Base string
	Tip  string
}

// MergeSession locates the agent session whose metrics a merging stamps. A
// zero ID means no session is recoverable, so no metrics are stamped.
type MergeSession struct {
	Agent AgentKind
	Cwd   string
	ID    string
}

// MergeDeps is the slice of Deps a merging needs.
type MergeDeps struct {
	RevParse             func(dir, ref string) (string, error)
	IsAncestor           func(dir, ancestor, descendant string) (bool, error)
	PatchesApplied       func(dir, upstream, base, branch string) (bool, error)
	CherryPickRange      func(dir, fromExclusive, toInclusive string) error
	CherryPickInProgress func(dir string) (bool, error)
	AppendTrailers       func(dir string, trailers ...git.Trailer) error
	MergedTickets        func(dir, featureBranch string) (map[string]bool, error)
}

// MergeParams describes one ticket merging onto FeatureBranch.
type MergeParams struct {
	FeatureWorktree string
	FeatureBranch   string
	TicketID        string
	TicketPath      string
	SourceRange     SourceRange
	// RecordedSHA is the SHA of a prior merging logged for this ticket, the
	// ladder's first rung; empty skips that rung.
	RecordedSHA string
	Session     MergeSession
}

// MergeResult is MergeTicket's outcome. SHA is empty for Conflicted.
type MergeResult struct {
	Outcome        MergeOutcome
	SHA            string
	TrailerValue   string
	MetricsStamped bool
}

func mergeDepsFor(d Deps) MergeDeps {
	return MergeDeps{
		RevParse:             d.RevParse,
		IsAncestor:           d.IsAncestor,
		PatchesApplied:       d.PatchesApplied,
		CherryPickRange:      d.CherryPickRange,
		CherryPickInProgress: d.CherryPickInProgress,
		AppendTrailers:       d.AppendTrailers,
		MergedTickets:        mergedTickets,
	}
}

// MergeTicket merges lp.SourceRange onto the feature branch: an already-applied
// check (see alreadyApplied), the pick, then stampMerged. A conflict is a
// result, never an error.
func MergeTicket(d MergeDeps, lp MergeParams) (MergeResult, error) {
	outcome, err := pickRange(d, lp)
	if err != nil {
		return MergeResult{}, err
	}
	switch outcome {
	case AlreadyApplied:
		sha, err := alreadyAppliedSHA(d, lp)
		if err != nil {
			return MergeResult{}, err
		}
		stamped, err := stampAppliedMetrics(lp)
		if err != nil {
			return MergeResult{}, err
		}
		return MergeResult{Outcome: AlreadyApplied, SHA: sha, TrailerValue: mergeTrailerValue(lp), MetricsStamped: stamped}, nil
	case Conflicted:
		return MergeResult{Outcome: Conflicted, TrailerValue: mergeTrailerValue(lp)}, nil
	}
	return stampMerged(d, lp)
}

// pickRange runs the already-applied ladder, then the cherry-pick core. It
// reports Merged for a clean pick that is not yet stamped.
func pickRange(d MergeDeps, lp MergeParams) (MergeOutcome, error) {
	applied, err := alreadyApplied(d, lp)
	if err != nil {
		return "", err
	}
	if applied {
		return AlreadyApplied, nil
	}
	conflicted, err := cherryPickCore(d, lp)
	if err != nil {
		return "", err
	}
	if conflicted {
		return Conflicted, nil
	}
	return Merged, nil
}

// ambiguousMergeError is a ticket whose trailer is on the feature branch while
// neither its recorded SHA nor its patches are: something merged under its
// name, but nothing proves it was these commits. Its Error is the pause reason.
type ambiguousMergeError struct {
	TicketID      string
	FeatureBranch string
	RecordedSHA   string
	SourceRange   SourceRange
}

func (e *ambiguousMergeError) Error() string {
	recorded := e.RecordedSHA
	if recorded == "" {
		recorded = "none"
	}
	return fmt.Sprintf("ticket %s has a merged trailer on %s, but neither its recorded SHA %s nor the patches of %s..%s are there; check by hand whether it really merged",
		e.TicketID, e.FeatureBranch, recorded, e.SourceRange.Base, e.SourceRange.Tip)
}

// alreadyApplied is the evidence ladder: recorded-SHA reachability, then
// patch-equivalence. A ticket trailer with neither is an ambiguousMergeError,
// not proof: a re-resolved conflict and a different commit reusing the ticket
// trailer look the same, and picking again could merge it twice. An unreadable
// trailer history is inconclusive, not an error: the pick itself still guards
// against a double merging.
func alreadyApplied(d MergeDeps, lp MergeParams) (bool, error) {
	if lp.RecordedSHA != "" {
		reachable, err := d.IsAncestor(lp.FeatureWorktree, lp.RecordedSHA, lp.FeatureBranch)
		if err != nil {
			return false, fmt.Errorf("checking merged commit reachability for ticket %s: %w", lp.TicketID, err)
		}
		if reachable {
			return true, nil
		}
	}
	applied, err := d.PatchesApplied(lp.FeatureWorktree, lp.FeatureBranch, lp.SourceRange.Base, lp.SourceRange.Tip)
	if err != nil {
		return false, fmt.Errorf("checking whether ticket %s is already applied: %w", lp.TicketID, err)
	}
	if applied {
		return true, nil
	}
	merged, err := d.MergedTickets(lp.FeatureWorktree, lp.FeatureBranch)
	if err != nil || !merged[lp.TicketID] {
		return false, nil
	}
	return false, &ambiguousMergeError{TicketID: lp.TicketID, FeatureBranch: lp.FeatureBranch, RecordedSHA: lp.RecordedSHA, SourceRange: lp.SourceRange}
}

// stampAppliedMetrics fills in the ticket's frontmatter metrics when commits
// were merged by hand, so a zero actual_cost doesn't read as never merged. The
// merged commit is already trailered, so nothing is amended. A nonzero cost
// means a prior merging stamped it; no recoverable session means nothing to
// stamp.
func stampAppliedMetrics(lp MergeParams) (bool, error) {
	if lp.TicketPath == "" || lp.Session.ID == "" {
		return false, nil
	}
	raw, err := os.ReadFile(lp.TicketPath)
	if err != nil {
		return false, fmt.Errorf("reading ticket %s: %w", lp.TicketID, err)
	}
	t, err := schema.ParseTicketFromRaw(string(raw), lp.TicketPath)
	if err != nil {
		return false, fmt.Errorf("parsing ticket %s: %w", lp.TicketID, err)
	}
	if t.ActualCost != 0 {
		return false, nil
	}
	_, _, _, ok, err := writeMergedMetrics(lp.Session.Agent, lp.Session.Cwd, lp.Session.ID, lp.TicketPath)
	if err != nil {
		return false, fmt.Errorf("writing merged metrics for ticket %s: %w", lp.TicketID, err)
	}
	return ok, nil
}

func alreadyAppliedSHA(d MergeDeps, lp MergeParams) (string, error) {
	if lp.RecordedSHA != "" {
		if reachable, err := d.IsAncestor(lp.FeatureWorktree, lp.RecordedSHA, lp.FeatureBranch); err == nil && reachable {
			return lp.RecordedSHA, nil
		}
	}
	sha, err := d.RevParse(lp.FeatureWorktree, "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolving already-applied commit on %s: %w", lp.FeatureBranch, err)
	}
	return sha, nil
}

// cherryPickCore picks lp.SourceRange onto lp.FeatureWorktree and reports
// whether it stopped on a conflict. On conflict the sequencer state is left
// intact for the caller to resolve or abort; any other failure is returned as
// an error.
func cherryPickCore(d MergeDeps, lp MergeParams) (conflicted bool, err error) {
	pickErr := d.CherryPickRange(lp.FeatureWorktree, lp.SourceRange.Base, lp.SourceRange.Tip)
	if pickErr == nil {
		return false, nil
	}
	inProgress, err := d.CherryPickInProgress(lp.FeatureWorktree)
	if err != nil {
		return false, fmt.Errorf("checking cherry-pick state onto %s: %w", lp.FeatureBranch, err)
	}
	if !inProgress {
		return false, fmt.Errorf("cherry-picking onto %s: %w", lp.FeatureBranch, pickErr)
	}
	return true, nil
}

// mergeTrailerValue is the ticket's canonical address, so identity survives a
// retarget; a ticket outside the tracker layout has none and keeps the legacy
// <featureBranch>/<id> form (mergedTickets reads both).
func mergeTrailerValue(lp MergeParams) string {
	if a, ok := tickets.AddressOfPath(lp.TicketPath, lp.TicketID); ok {
		return a.String()
	}
	return ticketTrailerValue(lp.FeatureBranch, lp.TicketID)
}

// stampMerged writes the merging session's metrics into the ticket's
// frontmatter, then stamps the ticket trailer — plus the metrics trailers when
// a session was recoverable — in a single amend, and resolves the merged SHA.
// Metrics are omitted, never fabricated, when no session is recoverable.
func stampMerged(d MergeDeps, lp MergeParams) (MergeResult, error) {
	contextWindow, elapsedSeconds, cost, hasMetrics, err := writeMergedMetrics(lp.Session.Agent, lp.Session.Cwd, lp.Session.ID, lp.TicketPath)
	if err != nil {
		return MergeResult{}, fmt.Errorf("writing merged metrics for ticket %s: %w", lp.TicketID, err)
	}

	trailerValue := mergeTrailerValue(lp)
	trailers := []git.Trailer{{Key: ticketTrailerKey, Value: trailerValue}}
	if hasMetrics {
		trailers = append(trailers,
			git.Trailer{Key: tokensTrailerKey, Value: strconv.Itoa(contextWindow)},
			git.Trailer{Key: elapsedTrailerKey, Value: strconv.Itoa(elapsedSeconds) + "s"},
			git.Trailer{Key: costTrailerKey, Value: tickets.FormatCost(cost)},
		)
	}
	if err := d.AppendTrailers(lp.FeatureWorktree, trailers...); err != nil {
		return MergeResult{}, fmt.Errorf("stamping trailers on merged commit: %w", err)
	}

	sha, err := d.RevParse(lp.FeatureWorktree, "HEAD")
	if err != nil {
		return MergeResult{}, fmt.Errorf("resolving merged commit on %s: %w", lp.FeatureBranch, err)
	}
	return MergeResult{Outcome: Merged, SHA: sha, TrailerValue: trailerValue, MetricsStamped: hasMetrics}, nil
}
