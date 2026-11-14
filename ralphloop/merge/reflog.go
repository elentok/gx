package merge

import (
	"fmt"

	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/tickets"
)

// ReflogRefusalReason says why an epic could not be matched to one entry.
type ReflogRefusalReason string

const (
	// ReasonNoTrailerCommits: the epic has no trailer commit on the target.
	// An empty set would match every entry, so callers handle this first.
	ReasonNoTrailerCommits ReflogRefusalReason = "no-trailer-commits"
	// ReasonNoEntry: no reflog entry holds all the epic's trailer commits.
	ReasonNoEntry ReflogRefusalReason = "no-entry"
	// ReasonMergedTwice: several entries each hold all of them.
	ReasonMergedTwice ReflogRefusalReason = "merged-twice"
	// ReasonSpread: the commits are spread over several entries.
	ReasonSpread ReflogRefusalReason = "spread"
	// ReasonSharedEntry: the one candidate entry also holds another epic's
	// trailer commit.
	ReasonSharedEntry ReflogRefusalReason = "shared-entry"
)

// ReflogRefusal is a deliberate "no answer", as opposed to a git failure.
type ReflogRefusal struct {
	Epic   string
	Reason ReflogRefusalReason
}

func (r *ReflogRefusal) Error() string {
	return fmt.Sprintf("epic %q has no single reflog range: %s", r.Epic, r.Reason)
}

// ReflogRange is the matched entry: Base is its old tip, Tip its new tip.
type ReflogRange struct{ Base, Tip string }

type reflogEntry struct {
	entry   history.ReflogEntry
	commits map[string][]string // trailer commit SHA -> epic parts of its values
}

// ReflogMatcher matches epics against the reflog of one target branch. Build
// it once per repo and target: the git calls grow with the number of entries,
// not with epics times entries.
type ReflogMatcher struct {
	tipCommits []history.TrailerCommit
	entries    []reflogEntry
}

// NewReflogMatcher reads the target's trailer commits and the commits of
// every forward reflog entry.
func NewReflogMatcher(dir, target string) (*ReflogMatcher, error) {
	tip, err := history.TrailerCommitsIn(dir, target, TicketTrailerKey)
	if err != nil {
		return nil, err
	}
	raw, err := history.ReflogEntries(dir, target)
	if err != nil {
		return nil, err
	}
	m := &ReflogMatcher{tipCommits: tip}
	for _, e := range raw {
		commits, err := history.TrailerCommitsIn(dir, e.Old+".."+e.New, TicketTrailerKey)
		if err != nil {
			return nil, err
		}
		info := reflogEntry{entry: e, commits: make(map[string][]string, len(commits))}
		for _, c := range commits {
			info.commits[c.SHA] = epicParts(c.Values)
		}
		m.entries = append(m.entries, info)
	}
	return m, nil
}

func epicParts(values []string) []string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		if e, _, ok := tickets.SplitTrailerValue(v); ok {
			parts = append(parts, e)
		}
	}
	return parts
}

// Match finds the one entry that holds every trailer commit of epic. The
// epic's name and its effective branch (the old name of a renamed epic) both
// count as naming it.
func (m *ReflogMatcher) Match(epic tickets.Epic) (ReflogRange, *ReflogRefusal) {
	names := map[string]bool{epic.Name: true, epic.Branch(): true}
	refuse := func(r ReflogRefusalReason) (ReflogRange, *ReflogRefusal) {
		return ReflogRange{}, &ReflogRefusal{Epic: epic.Name, Reason: r}
	}
	own := map[string]bool{}
	for _, c := range m.tipCommits {
		for _, p := range epicParts(c.Values) {
			if names[p] {
				own[c.SHA] = true
			}
		}
	}
	if len(own) == 0 {
		return refuse(ReasonNoTrailerCommits)
	}
	var full []reflogEntry
	touching := 0
	for _, e := range m.entries {
		held := 0
		for sha := range own {
			if _, ok := e.commits[sha]; ok {
				held++
			}
		}
		if held > 0 {
			touching++
		}
		if held == len(own) {
			full = append(full, e)
		}
	}
	switch {
	case len(full) > 1:
		return refuse(ReasonMergedTwice)
	case len(full) == 0 && touching > 1:
		return refuse(ReasonSpread)
	case len(full) == 0:
		return refuse(ReasonNoEntry)
	}
	for _, parts := range full[0].commits {
		for _, p := range parts {
			if !names[p] {
				return refuse(ReasonSharedEntry)
			}
		}
	}
	return ReflogRange{Base: full[0].entry.Old, Tip: full[0].entry.New}, nil
}
