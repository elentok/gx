package epics

import (
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// DiffRefs answers the two git questions behind an unmerged epic's View diff.
type DiffRefs interface {
	// EpicBranchAhead returns the epic's derived base and how many commits its
	// branch has past that base. It errors when the project has no repo, the
	// base cannot be derived or the branch does not exist.
	EpicBranchAhead(project string, epics []tickets.Epic, e tickets.Epic) (base string, ahead int, err error)
}

// viewDiff is the View diff part of an epic response.
type viewDiff struct {
	HasCommitsAhead bool
	Shows           bool
	Base, Tip       string
}

// viewDiffOf computes whether e shows View diff and which refs the link opens.
// A map epic never does: map_done copies the impl epic's merged_at onto it.
// A git failure leaves both booleans false.
func (s *Epics) viewDiffOf(project string, epics []tickets.Epic, e tickets.Epic) viewDiff {
	if e.IsMap {
		return viewDiff{}
	}
	if !e.MergedAt.IsZero() {
		// Both SHAs empty is an unstamped range, which still shows the link.
		empty := e.RangeBase == e.RangeTip && e.RangeBase != ""
		return viewDiff{Shows: !empty, Base: e.RangeBase, Tip: e.RangeTip}
	}
	if s.Diff == nil {
		return viewDiff{}
	}
	base, ahead, err := s.Diff.EpicBranchAhead(project, epics, e)
	if err != nil {
		s.Log.Debug("view diff refs", "epic", e.Name, "err", err)
		return viewDiff{}
	}
	return viewDiff{HasCommitsAhead: ahead > 0, Shows: ahead > 0, Base: base, Tip: e.Branch()}
}

// applyViewDiff copies v onto a response's View diff part.
func applyViewDiff(d *wire.ViewDiff, v viewDiff) {
	d.ShowsViewDiff = v.Shows
	if v.Shows {
		d.DiffBase, d.DiffTip = v.Base, v.Tip
	}
}
