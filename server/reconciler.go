package server

import (
	"sync"

	"github.com/elentok/gx/git"
)

// Reconciler is the server's startup and rescan repair of merge state: late
// finishes, hand-merged epics, parent defects, missing trunks and the removal
// of merged worktrees. It is split from Server in place; the embedded *Server is
// the way into the rest of the server until the package move cuts it.
type Reconciler struct {
	*Server
	defectScan   sync.Mutex
	lateFinishes inFlight
	trunkMissing missingTrunks
	mergedProbe  func(repo, ancestor, descendant string) (bool, error) // the poll-time ancestry check; swapped in tests
}

func newReconciler(s *Server) *Reconciler {
	return &Reconciler{Server: s, mergedProbe: git.IsAncestor}
}
