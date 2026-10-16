package cmd

import (
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui/nav"
)

func TestTabTarget_AgentsOpensFromABareRepo(t *testing.T) {
	t.Parallel()
	bare := testutil.TempBareRepo(t)

	repo, root, err := tabTarget(bare, nav.TabAgents)
	if err != nil {
		t.Fatalf("gx agents from a bare repo: %v", err)
	}
	if !repo.IsBare || root != repo.Root {
		t.Fatalf("got repo %+v and root %q, want the bare repo as both", repo, root)
	}

	_, _, err = tabTarget(bare, nav.TabTickets)
	if err == nil || !strings.Contains(err.Error(), "must be run from a regular repo or linked worktree") {
		t.Fatalf("gx tickets from a bare repo: got %v, want the worktree error", err)
	}
}
