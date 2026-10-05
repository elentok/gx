package ralphloop

import (
	"fmt"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// Landing is whether a ticket's iteration commits made it onto the feature
// branch. recoverable/unrecoverable keep the done-ticket classifier's
// vocabulary without the "done" prefix.
type Landing string

const (
	LandingLanded        Landing = "landed"
	LandingRecoverable   Landing = "recoverable"
	LandingUnrecoverable Landing = "unrecoverable"
	// LandingNotExpected covers commitless and never-iterated tickets.
	LandingNotExpected Landing = "not-expected"
	// LandingUnknown means a check couldn't run — never a stand-in for "not
	// landed".
	LandingUnknown Landing = "unknown"
)

// Evidence names the ladder rung that proved a landing.
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

// TicketVerification is one ticket's landing truth. Landing and Leftovers are
// orthogonal: a claimed ticket can have leftovers by design.
type TicketVerification struct {
	ID        string
	Status    string
	Landing   Landing
	Evidence  Evidence
	SHA       string
	Leftovers Leftovers
	Unknown   bool

	// err is why Landing is unknown when a check failed outright (as opposed
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
	// TabList may be nil (herdr unavailable): every Leftovers.Tab is then nil.
	TabList func(workspaceID string) ([]herdr.Tab, error)
	// LandedTickets defaults to landedTickets (the trailer rung).
	LandedTickets func(dir, featureBranch string) (map[string]bool, error)
}

func (d Deps) verifyDeps() VerifyDeps {
	return VerifyDeps{
		IsAncestor:     d.IsAncestor,
		MergeBase:      d.MergeBase,
		PatchesApplied: d.PatchesApplied,
		RevParse:       d.RevParse,
		WorktreeExists: d.WorktreeExists,
		TabList:        d.TabList,
	}
}

// DefaultVerifyDeps wires VerifyDeps to the real git and herdr packages, for
// one-shot CLI callers.
func DefaultVerifyDeps() VerifyDeps {
	return DefaultDeps().verifyDeps()
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

// VerifyEpic is the single landing-truth gatherer: for each ticket it climbs
// a three-rung ladder — the recorded SHA still reachable, patch-equivalent
// commits on the feature branch, the Ralph-Loop-Ticket trailer — and reports
// the rung that answered plus any leftover tab/worktree/branch. Presence is
// checked against the SHA recorded on the ticket's latest cherry-picked
// event, not the iteration branch: CherryPickRange creates fresh commits, and
// a later rebase can stale the SHA harmlessly, so later rungs exist to avoid
// misclassifying landed work as recoverable (and re-cherry-picking it).
func VerifyEpic(vd VerifyDeps, p VerifyParams) ([]TicketVerification, error) {
	if len(p.Tickets) == 0 {
		return nil, nil
	}

	var live map[string]bool
	if vd.TabList != nil {
		if tabs, err := vd.TabList(p.WorkspaceID); err == nil {
			live = make(map[string]bool, len(tabs))
			for _, tab := range tabs {
				live[iterationKey(p.Epic, tab.Label)] = true
			}
		}
	}

	landedFn := vd.LandedTickets
	if landedFn == nil {
		landedFn = landedTickets
	}
	landed, landedErr := landedFn(p.FeatureWorktree, p.Epic)

	paths := reconcilePaths{FeatureWorktree: p.FeatureWorktree, WorktreeDir: p.WorktreeDir}
	out := make([]TicketVerification, 0, len(p.Tickets))
	for _, t := range p.Tickets {
		out = append(out, verifyTicket(vd, paths, p.Epic, t, p.Events, live, landed, landedErr))
	}
	return out, nil
}

// verifyTicket verifies one ticket. live nil means herdr did not answer;
// landedErr non-nil means the trailer rung couldn't run, which only matters
// when no earlier rung already proved the landing.
func verifyTicket(vd VerifyDeps, paths reconcilePaths, featureBranch string, t tickets.Ticket, events []Event, live, landed map[string]bool, landedErr error) TicketVerification {
	v := TicketVerification{ID: t.Identifier, Status: t.Status, Evidence: EvidenceNone}
	fail := func(err error) TicketVerification {
		v.Landing, v.Unknown, v.err = LandingUnknown, true, err
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
		v.Landing = LandingNotExpected
		return v
	}

	sha := latestCherryPickedSHA(events, t.Identifier)
	present := false
	if sha != "" {
		ok, err := vd.IsAncestor(paths.FeatureWorktree, sha, featureBranch)
		if err != nil {
			return fail(fmt.Errorf("checking landed commit reachability: %w", err))
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
			return fail(fmt.Errorf("checking patch-equivalent landed commits: %w", err))
		}
		if ok {
			present, v.Evidence, v.SHA = true, EvidencePatchID, sha
		}
	}

	// The trailer survives a rebase where the commit was also re-resolved
	// during a conflict (changing both hash and patch-id), and doesn't depend
	// on the iteration branch surviving.
	if !present {
		if landed[t.Identifier] {
			present, v.Evidence, v.SHA = true, EvidenceTrailer, sha
		} else if landedErr != nil {
			v.Landing, v.Unknown = LandingUnknown, true
			return v
		}
	}

	switch {
	case present:
		v.Landing = LandingLanded
	case hasBranch:
		v.Landing = LandingRecoverable
	case !t.IsDone() && sha == "":
		v.Landing = LandingNotExpected
	default:
		v.Landing = LandingUnrecoverable
	}
	return v
}
