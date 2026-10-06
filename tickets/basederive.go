package tickets

import (
	"cmp"
	"fmt"
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
// parking that.
func DeriveRootBase(project string, epics []Epic, epic string, t Ticket) (string, error) {
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
	return "", fmt.Errorf("ambiguous base: unlanded blockers on %s", strings.Join(branches, ", "))
}

// unlandedBranch is the feature branch of the epic holding key, or "" once
// key (and its fork subtree) has landed.
func unlandedBranch(g *projectGraph, key string) string {
	if !g.blocking(key) {
		return ""
	}
	return key[:strings.Index(key, "/")]
}
