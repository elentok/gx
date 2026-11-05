package sched

import (
	"errors"
	"fmt"

	"github.com/elentok/gx/git"
	srvrecovery "github.com/elentok/gx/server/recovery"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketgit"
)

// ResolveBase derives where t starts from. ref is the branch the root epic's
// feature branch is created from (the epic's refs, not t's), with its
// "ref@sha" claim stamp; leaf is the
// unmerged sibling whose branch t starts from instead of the feature tip. A
// commitless ticket reads at the feature tip, so everything comes back empty.
func (s *Scheduler) ResolveBase(addr tickets.Address, t tickets.Ticket, repo string, epics []tickets.Epic) (ref, stamp, leaf string, err error) {
	if t.Commitless {
		return "", "", "", nil
	}
	if leaf, err = tickets.DeriveLeafBase(addr.Project, epics, addr.Epic, t); err != nil {
		return "", "", "", err
	}
	_, ref, _, err = s.Merger.EpicRefs(addr.Project, epics, addr.Epic, repo)
	if err != nil {
		return "", "", "", err
	}
	sha, err := git.RevParse(repo, ref)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	// A feature branch that already exists is adopted, not recreated; its
	// merge-base stays the branch's true start even after ref moves on.
	switch _, err := git.RevParse(repo, "refs/heads/"+addr.Epic); {
	case err == nil:
		if sha, err = git.MergeBase(repo, "refs/heads/"+addr.Epic, ref); err != nil {
			return "", "", "", fmt.Errorf("merge-base of %s and %s: %w", addr.Epic, ref, err)
		}
	case !errors.Is(err, git.ErrRefNotFound):
		return "", "", "", fmt.Errorf("resolve %s: %w", addr.Epic, err)
	}
	return ref, ref + "@" + sha, leaf, nil
}

// gitIsAncestor is swapped in tests to count git calls.
var gitIsAncestor = git.IsAncestor

// RootRefs is where a root's feature branch starts (base: the claim base and
// the commitless ref) and where it merges (target). Trunk is read on every
// call, so a merging uses the trunk current then; nothing records it at claim.
// An explicit base: moves only the start; a derived stacked root also merges
// on its blocker's branch until that branch reaches trunk.
func (s *Scheduler) RootRefs(project string, epics []tickets.Epic, epic string, t tickets.Ticket, repo string) (base, target string, err error) {
	dir, err := s.Projects.Dir(project)
	if err != nil {
		return "", "", err
	}
	trunk, trunkErr := ticketgit.Trunk(dir, repo)
	derived, err := tickets.DeriveRootBase(project, epics, epic, t, s.Projects.MergingOnMerged(project))
	if err != nil {
		return "", "", err
	}
	targetsTrunk := derived == "" || explicitBase(epics, epic, t)
	switch {
	case targetsTrunk && trunkErr != nil:
		return "", "", trunkErr
	case derived == "":
		return trunk, trunk, nil
	case targetsTrunk:
		return derived, trunk, nil
	case trunkErr != nil:
		// Only a root targeting trunk needs it; a blocker's branch can't have
		// merged on a trunk that isn't there.
		return derived, derived, nil
	}
	ref := retargetIfMerged(derived, trunk, func(a, d string) (bool, error) { return gitIsAncestor(repo, a, d) })
	return ref, ref, nil
}

// MergeTarget is the target of the root epic addr belongs to.
func (s *Scheduler) MergeTarget(addr tickets.Address) (string, error) {
	dir, repo, err := s.Projects.Of(addr.Project)
	if err != nil {
		return "", err
	}
	epics, err := tickets.LoadResolved(addr.Project, dir)
	if err != nil {
		return "", err
	}
	_, _, target, err := s.Merger.EpicRefs(addr.Project, epics, addr.Epic, repo)
	return target, err
}

// MergeTargetTip is the SHA addr's merge target points at now.
func (s *Scheduler) MergeTargetTip(addr tickets.Address, repo string) (string, error) {
	target, err := s.MergeTarget(addr)
	if err != nil {
		return "", err
	}
	return git.RevParse(repo, target)
}

// explicitBase reports whether t's base comes from a base: field (the
// ticket's or its epic's) rather than from its blockers.
func explicitBase(epics []tickets.Epic, epic string, t tickets.Ticket) bool {
	if t.Base != "" {
		return true
	}
	for _, e := range epics {
		if e.Name == epic {
			return e.Base != ""
		}
	}
	return false
}

// retargetIfMerged is derived, never stored: once the blocker's branch is
// reachable from trunk the dependent targets trunk again.
// An unresolvable branch counts as not merged; never retarget on a guess.
func retargetIfMerged(ref, trunk string, isAncestor func(ancestor, descendant string) (bool, error)) string {
	if merged, err := isAncestor(ref, trunk); err == nil && merged {
		return trunk
	}
	return ref
}

// CommitlessRef is the ref a commitless ticket's detached worktree starts at:
// where its blockers resolve to.
func (s *Scheduler) CommitlessRef(addr tickets.Address, t tickets.Ticket, repo string, epics []tickets.Epic) (string, error) {
	if ref, ok := srvrecovery.InvestigateRef(repo, addr, t, epics); ok && t.Type == string(schema.TypeInvestigate) {
		return ref, nil
	}
	ref, _, err := s.RootRefs(addr.Project, epics, addr.Epic, t, repo)
	return ref, err
}
