package ralphloop

import (
	"fmt"
	"os"
	"strconv"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// LandOutcome is what LandTicket did with a ticket's commits.
type LandOutcome string

const (
	// Landed: the source range was cherry-picked and stamped.
	Landed LandOutcome = "landed"
	// AlreadyApplied: the evidence ladder found the commits on the feature
	// branch already; nothing was picked and no trailer was re-stamped.
	AlreadyApplied LandOutcome = "already_applied"
	// Conflicted: the pick stopped on a conflict. Sequencer state is left
	// intact for the caller to resolve or abort; nothing was stamped.
	Conflicted LandOutcome = "conflicted"
)

// SourceRange is the commit range base..Tip (base exclusive) a landing picks.
type SourceRange struct {
	Base string
	Tip  string
}

// LandSession locates the agent session whose metrics a landing stamps. A
// zero ID means no session is recoverable, so no metrics are stamped.
type LandSession struct {
	Agent AgentKind
	Cwd   string
	ID    string
}

// LandDeps is the slice of Deps a landing needs.
type LandDeps struct {
	RevParse             func(dir, ref string) (string, error)
	IsAncestor           func(dir, ancestor, descendant string) (bool, error)
	PatchesApplied       func(dir, upstream, base, branch string) (bool, error)
	CherryPickRange      func(dir, fromExclusive, toInclusive string) error
	CherryPickInProgress func(dir string) (bool, error)
	AppendTrailers       func(dir string, trailers ...git.Trailer) error
	LandedTickets        func(dir, featureBranch string) (map[string]bool, error)
}

// LandParams describes one ticket landing onto FeatureBranch.
type LandParams struct {
	FeatureWorktree string
	FeatureBranch   string
	TicketID        string
	TicketPath      string
	SourceRange     SourceRange
	// RecordedSHA is the SHA of a prior landing logged for this ticket, the
	// ladder's first rung; empty skips that rung.
	RecordedSHA string
	Session     LandSession
}

// LandResult is LandTicket's outcome. SHA is empty for Conflicted.
type LandResult struct {
	Outcome        LandOutcome
	SHA            string
	TrailerValue   string
	MetricsStamped bool
}

func landDepsFor(d Deps) LandDeps {
	return LandDeps{
		RevParse:             d.RevParse,
		IsAncestor:           d.IsAncestor,
		PatchesApplied:       d.PatchesApplied,
		CherryPickRange:      d.CherryPickRange,
		CherryPickInProgress: d.CherryPickInProgress,
		AppendTrailers:       d.AppendTrailers,
		LandedTickets:        landedTickets,
	}
}

// LandTicket lands lp.SourceRange onto the feature branch: an already-applied
// check (see alreadyApplied), the pick, then stampLanded. A conflict is a
// result, never an error.
func LandTicket(d LandDeps, lp LandParams) (LandResult, error) {
	outcome, err := pickRange(d, lp)
	if err != nil {
		return LandResult{}, err
	}
	switch outcome {
	case AlreadyApplied:
		sha, err := alreadyAppliedSHA(d, lp)
		if err != nil {
			return LandResult{}, err
		}
		stamped, err := stampAppliedMetrics(lp)
		if err != nil {
			return LandResult{}, err
		}
		return LandResult{Outcome: AlreadyApplied, SHA: sha, TrailerValue: landTrailerValue(lp), MetricsStamped: stamped}, nil
	case Conflicted:
		return LandResult{Outcome: Conflicted, TrailerValue: landTrailerValue(lp)}, nil
	}
	return stampLanded(d, lp)
}

// pickRange runs the already-applied ladder, then the cherry-pick core. It
// reports Landed for a clean pick that is not yet stamped.
func pickRange(d LandDeps, lp LandParams) (LandOutcome, error) {
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
	return Landed, nil
}

// alreadyApplied is the three-rung evidence ladder: recorded-SHA
// reachability, then patch-equivalence, then the ticket trailer. Each later
// rung covers a rewrite the earlier ones cannot see (a rebase changes hashes;
// a manually re-resolved conflict also changes patch-ids, but never the
// commit message). An unreadable trailer history is inconclusive, not an
// error: the pick itself still guards against a double landing.
func alreadyApplied(d LandDeps, lp LandParams) (bool, error) {
	if lp.RecordedSHA != "" {
		reachable, err := d.IsAncestor(lp.FeatureWorktree, lp.RecordedSHA, lp.FeatureBranch)
		if err != nil {
			return false, fmt.Errorf("checking landed commit reachability for ticket %s: %w", lp.TicketID, err)
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
	landed, err := d.LandedTickets(lp.FeatureWorktree, lp.FeatureBranch)
	if err != nil {
		return false, nil
	}
	return landed[lp.TicketID], nil
}

// stampAppliedMetrics fills in the ticket's frontmatter metrics when commits
// were landed by hand, so a zero actual_cost doesn't read as never landed. The
// landed commit is already trailered, so nothing is amended. A nonzero cost
// means a prior landing stamped it; no recoverable session means nothing to
// stamp.
func stampAppliedMetrics(lp LandParams) (bool, error) {
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
	_, _, _, ok, err := writeLandedMetrics(lp.Session.Agent, lp.Session.Cwd, lp.Session.ID, lp.TicketPath)
	if err != nil {
		return false, fmt.Errorf("writing landed metrics for ticket %s: %w", lp.TicketID, err)
	}
	return ok, nil
}

func alreadyAppliedSHA(d LandDeps, lp LandParams) (string, error) {
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
func cherryPickCore(d LandDeps, lp LandParams) (conflicted bool, err error) {
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

// landTrailerValue is the ticket's canonical address, so identity survives a
// retarget; a ticket outside the tracker layout has none and keeps the legacy
// <featureBranch>/<id> form (landedTickets reads both).
func landTrailerValue(lp LandParams) string {
	if a, ok := tickets.AddressOfPath(lp.TicketPath, lp.TicketID); ok {
		return a.String()
	}
	return ticketTrailerValue(lp.FeatureBranch, lp.TicketID)
}

// stampLanded writes the landing session's metrics into the ticket's
// frontmatter, then stamps the ticket trailer — plus the metrics trailers when
// a session was recoverable — in a single amend, and resolves the landed SHA.
// Metrics are omitted, never fabricated, when no session is recoverable.
func stampLanded(d LandDeps, lp LandParams) (LandResult, error) {
	contextWindow, elapsedSeconds, cost, hasMetrics, err := writeLandedMetrics(lp.Session.Agent, lp.Session.Cwd, lp.Session.ID, lp.TicketPath)
	if err != nil {
		return LandResult{}, fmt.Errorf("writing landed metrics for ticket %s: %w", lp.TicketID, err)
	}

	trailerValue := landTrailerValue(lp)
	trailers := []git.Trailer{{Key: ticketTrailerKey, Value: trailerValue}}
	if hasMetrics {
		trailers = append(trailers,
			git.Trailer{Key: tokensTrailerKey, Value: strconv.Itoa(contextWindow)},
			git.Trailer{Key: elapsedTrailerKey, Value: strconv.Itoa(elapsedSeconds) + "s"},
			git.Trailer{Key: costTrailerKey, Value: tickets.FormatCost(cost)},
		)
	}
	if err := d.AppendTrailers(lp.FeatureWorktree, trailers...); err != nil {
		return LandResult{}, fmt.Errorf("stamping trailers on landed commit: %w", err)
	}

	sha, err := d.RevParse(lp.FeatureWorktree, "HEAD")
	if err != nil {
		return LandResult{}, fmt.Errorf("resolving landed commit on %s: %w", lp.FeatureBranch, err)
	}
	return LandResult{Outcome: Landed, SHA: sha, TrailerValue: trailerValue, MetricsStamped: hasMetrics}, nil
}
