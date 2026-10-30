package ralphloop

import (
	"fmt"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets"
)

// Merging is whether a ticket's iteration commits made it onto the feature
// branch. recoverable/unrecoverable keep the done-ticket classifier's
// vocabulary without the "done" prefix.
type Merging string

const (
	MergingMerged        Merging = "merged"
	MergingRecoverable   Merging = "recoverable"
	MergingUnrecoverable Merging = "unrecoverable"
	// MergingNotExpected covers commitless and never-iterated tickets.
	MergingNotExpected Merging = "not-expected"
	// MergingUnknown means a check couldn't run — never a stand-in for "not
	// merged".
	MergingUnknown Merging = "unknown"
)

// Evidence names the ladder rung that proved a merging.
type Evidence string

const (
	EvidenceSHA     Evidence = "sha"
	EvidencePatchID Evidence = "patch-id"
	EvidenceTrailer Evidence = "trailer"
	EvidenceNone    Evidence = "none"
)

// Leftovers reports iteration state still around for a ticket. A nil field
// means the check couldn't get an answer (herdr down for Tab), never false.
type Leftovers struct {
	Tab      *bool
	Worktree *bool
	Branch   *bool
}

// TicketVerification is one ticket's merging truth. Merging and Leftovers are
// orthogonal: a claimed ticket can have leftovers by design.
type TicketVerification struct {
	ID        string
	Status    string
	Merging   Merging
	Evidence  Evidence
	SHA       string
	Leftovers Leftovers
	Unknown   bool

	// err is why Merging is unknown when a check failed outright (as opposed
	// to a degraded-but-non-fatal source, see verifyTicket's trailer rung).
	err error
}

// VerifyDeps is the read-only slice of Deps VerifyEpic needs: no write
// operations by construction.
type VerifyDeps struct {
	IsAncestor     func(dir, ancestor, descendant string) (bool, error)
	MergeBase      func(dir, refA, refB string) (string, error)
	PatchesApplied func(dir, upstream, base, branch string) (bool, error)
	RevParse       func(dir, ref string) (string, error)
	WorktreeExists func(path string) (bool, error)
	// ListSessions may be nil (no runner): every Leftovers.Tab is then nil.
	ListSessions func(epic string) ([]agentrunner.Session, error)
	// MergedTickets defaults to mergedTickets (the trailer rung).
	MergedTickets func(dir, featureBranch string) (map[string]bool, error)
}

func (d Deps) verifyDeps() VerifyDeps {
	vd := VerifyDeps{
		IsAncestor:     d.IsAncestor,
		MergeBase:      d.MergeBase,
		PatchesApplied: d.PatchesApplied,
		RevParse:       d.RevParse,
		WorktreeExists: d.WorktreeExists,
	}
	if d.Runner != nil {
		vd.ListSessions = d.Runner.List
	}
	return vd
}

// DefaultVerifyDeps wires VerifyDeps to the real git package and lists
// sessions on runner (nil: leftovers unknown).
func DefaultVerifyDeps(runner agentrunner.Runner) VerifyDeps {
	d := DefaultDeps()
	d.Runner = runner
	return d.verifyDeps()
}

// VerifyParams selects what VerifyEpic checks. Epic is also the feature
// branch name.
type VerifyParams struct {
	Epic            string
	FeatureWorktree string
	WorktreeDir     string
	WorkspaceID     string
	Tickets         []tickets.Ticket
	Events          []Event
}

// VerifyEpic is the single merging-truth gatherer: for each ticket it climbs
// a three-rung ladder — the recorded SHA still reachable, patch-equivalent
// commits on the feature branch, the Ralph-Loop-Ticket trailer — and reports
// the rung that answered plus any leftover tab/worktree/branch. Presence is
// checked against the SHA recorded on the ticket's latest cherry-picked
// event, not the iteration branch: CherryPickRange creates fresh commits, and
// a later rebase can stale the SHA harmlessly, so later rungs exist to avoid
// misclassifying merged work as recoverable (and re-cherry-picking it).
func VerifyEpic(vd VerifyDeps, p VerifyParams) ([]TicketVerification, error) {
	if len(p.Tickets) == 0 {
		return nil, nil
	}

	var live map[string]bool
	if vd.ListSessions != nil {
		if sessions, err := vd.ListSessions(p.Epic); err == nil {
			live = make(map[string]bool, len(sessions))
			for _, s := range sessions {
				live[iterationKey(p.Epic, s.Label)] = true
			}
		}
	}

	mergedFn := vd.MergedTickets
	if mergedFn == nil {
		mergedFn = mergedTickets
	}
	merged, mergedErr := mergedFn(p.FeatureWorktree, p.Epic)

	paths := reconcilePaths{FeatureWorktree: p.FeatureWorktree, WorktreeDir: p.WorktreeDir}
	out := make([]TicketVerification, 0, len(p.Tickets))
	for _, t := range p.Tickets {
		out = append(out, verifyTicket(vd, paths, p.Epic, t, p.Events, live, merged, mergedErr))
	}
	return out, nil
}

// verifyTicket verifies one ticket. live nil means herdr did not answer;
// mergedErr non-nil means the trailer rung couldn't run, which only matters
// when no earlier rung already proved the merging.
func verifyTicket(vd VerifyDeps, paths reconcilePaths, featureBranch string, t tickets.Ticket, events []Event, live, merged map[string]bool, mergedErr error) TicketVerification {
	v := TicketVerification{ID: t.Identifier, Status: t.Status, Evidence: EvidenceNone}
	fail := func(err error) TicketVerification {
		v.Merging, v.Unknown, v.err = MergingUnknown, true, err
		return v
	}

	branch := iterBranch(featureBranch, t.Identifier)
	_, revErr := vd.RevParse(paths.FeatureWorktree, branch)
	hasBranch := revErr == nil
	v.Leftovers.Branch = &hasBranch

	hasWorktree, err := vd.WorktreeExists(iterationWorktreePath(paths.WorktreeDir, featureBranch, t.Identifier))
	if err != nil {
		return fail(fmt.Errorf("checking leftover worktree: %w", err))
	}
	v.Leftovers.Worktree = &hasWorktree

	if live != nil {
		hasTab := live[iterationKey(featureBranch, iterLabel(featureBranch, t.Identifier))]
		v.Leftovers.Tab = &hasTab
	}

	if t.Commitless {
		v.Merging = MergingNotExpected
		return v
	}

	sha := latestCherryPickedSHA(events, t.Identifier)
	present := false
	if sha != "" {
		ok, err := vd.IsAncestor(paths.FeatureWorktree, sha, featureBranch)
		if err != nil {
			return fail(fmt.Errorf("checking merged commit reachability: %w", err))
		}
		if ok {
			present, v.Evidence, v.SHA = true, EvidenceSHA, sha
		}
	}

	// A missing SHA also happens harmlessly after a rebase or a lost event, so
	// before calling it a real gap check whether the iteration branch's
	// content is already on the feature branch under different hashes.
	if !present && hasBranch {
		base, err := vd.MergeBase(paths.FeatureWorktree, branch, featureBranch)
		if err != nil {
			return fail(fmt.Errorf("resolving merge-base for patch-equivalence check: %w", err))
		}
		ok, err := vd.PatchesApplied(paths.FeatureWorktree, featureBranch, base, branch)
		if err != nil {
			return fail(fmt.Errorf("checking patch-equivalent merged commits: %w", err))
		}
		if ok {
			present, v.Evidence, v.SHA = true, EvidencePatchID, sha
		}
	}

	// The trailer survives a rebase where the commit was also re-resolved
	// during a conflict (changing both hash and patch-id), and doesn't depend
	// on the iteration branch surviving.
	if !present {
		if merged[t.Identifier] {
			present, v.Evidence, v.SHA = true, EvidenceTrailer, sha
		} else if mergedErr != nil {
			v.Merging, v.Unknown = MergingUnknown, true
			return v
		}
	}

	switch {
	case present:
		v.Merging = MergingMerged
	case hasBranch:
		v.Merging = MergingRecoverable
	case !t.IsDone() && sha == "":
		v.Merging = MergingNotExpected
	default:
		v.Merging = MergingUnrecoverable
	}
	return v
}
