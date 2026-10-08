package tickets

import (
	"cmp"
	"slices"
	"strings"
)

// DeriveRootBase returns the branch the root epic's feature branch starts from;
// "" means trunk. t is the ticket being claimed, in epic.
//
// An explicit base: wins (the ticket's, else the epic's): a node ref gives its
// epic's feature branch while that node is unlanded and trunk once it lands,
// and anything that is not a node ref is a raw branch. Otherwise the base is
// the one unlanded commitful blocker's feature branch, or trunk when there is
// none. Two or more unlanded blockers are ambiguous; see the leaf tickets for
// parking that. Under the on-landed landing policy (onLanded) a blocker
// releases its dependents only once it has landed, so a derived base is always
// trunk; only an explicit base: can point elsewhere.
func DeriveRootBase(project string, epics []Epic, epic string, t Ticket, onLanded bool) (string, error) {
	g := newProjectGraph(project, epics)
	var e Epic
	for _, c := range epics {
		if c.Name == epic {
			e = c
		}
	}
	if base := cmp.Or(t.Base, e.Base); base != "" {
		key, err := g.resolve(epic, base)
		if err != nil {
			return base, nil
		}
		return unlandedBranch(g, key), nil
	}
	if onLanded {
		return "", nil
	}
	var branches []string
	for _, ref := range append(qualifiedRefs(t.BlockedBy), qualifiedRefs(e.BlockedBy)...) {
		key, err := g.resolve(epic, ref)
		if err != nil || g.tickets[key].Commitless {
			continue
		}
		if b := unlandedBranch(g, key); b != "" && !slices.Contains(branches, b) {
			branches = append(branches, b)
		}
	}
	switch len(branches) {
	case 0:
		return "", nil
	case 1:
		return branches[0], nil
	}
	return "", &AmbiguousBaseError{Blockers: branches}
}

// AmbiguousBaseError means two or more unlanded commitful blockers could each
// be the base and the ticket names none with base:. Blockers are what the
// person has to choose between.
type AmbiguousBaseError struct{ Blockers []string }

func (e *AmbiguousBaseError) Error() string {
	return "ambiguous base: unlanded blockers " + strings.Join(e.Blockers, ", ")
}

// DeriveLeafBase returns the identifier of the sibling whose iteration branch
// the leaf t starts from; "" means the epic's feature branch tip, which is
// also where a commitless ticket reads. Only same-epic blockers count: an
// unlanded commitful one is the base, and two or more are ambiguous. An
// explicit base: on the ticket opts out of derivation.
func DeriveLeafBase(project string, epics []Epic, epic string, t Ticket) (string, error) {
	if t.Commitless || t.Base != "" {
		return "", nil
	}
	g := newProjectGraph(project, epics)
	var siblings []string
	for _, ref := range t.BlockedBy {
		key, err := g.resolve(epic, ref)
		if err != nil || !strings.HasPrefix(key, epic+"/") || g.tickets[key].Commitless || !g.blocking(key) {
			continue
		}
		if id := g.tickets[key].Identifier; !slices.Contains(siblings, id) {
			siblings = append(siblings, id)
		}
	}
	switch len(siblings) {
	case 0:
		return "", nil
	case 1:
		return siblings[0], nil
	}
	return "", &AmbiguousBaseError{Blockers: siblings}
}

// unlandedBranch is the feature branch of the epic holding key, or "" once
// key (and its fork subtree) has landed.
func unlandedBranch(g *projectGraph, key string) string {
	if !g.blocking(key) {
		return ""
	}
	return key[:strings.Index(key, "/")]
}
