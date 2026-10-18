package server

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestEpicMergeTarget_CachesPerEpicUntilTicketsOrRefsChange(t *testing.T) {
	s, repo := rootRefsFixture(t)
	done := func(name string, blockedBy ...string) tickets.Epic {
		return tickets.Epic{Name: name, Path: "/store/proj/" + name, BlockedBy: blockedBy,
			Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "done"}}}
	}
	s.idx = &index{loaded: map[string][]tickets.Epic{}}
	dir, _, err := s.projectOf("proj")
	if err != nil {
		t.Fatal(err)
	}
	load := func(epics ...tickets.Epic) { s.idx.loaded[dir] = epics }
	load(done("a"), done("b", "a/01"))

	calls := 0
	orig := gitIsAncestor
	gitIsAncestor = func(repo, a, d string) (bool, error) { calls++; return orig(repo, a, d) }
	t.Cleanup(func() { gitIsAncestor = orig })

	key := tickets.Address{Project: "proj", Epic: "b"}
	if got := s.epicMergeTarget(key); got != "a" {
		t.Fatalf("target = %q, want a", got)
	}
	first := calls
	if first == 0 {
		t.Fatal("first resolve made no ancestry call; the fixture doesn't exercise git")
	}
	if got := s.epicMergeTarget(key); got != "a" || calls != first {
		t.Errorf("repeat: target %q, %d new git calls, want a and none", got, calls-first)
	}

	// A trunk move lands the blocker, so the target is fresh trunk again.
	runGit(t, repo, "branch", "-f", "develop", "a")
	if got := s.epicMergeTarget(key); got != "develop" {
		t.Errorf("after trunk move: target %q, want develop", got)
	}

	// A rescan (new epic slice) re-resolves.
	before := calls
	load(done("a"), done("b", "a/01"))
	s.epicMergeTarget(key)
	if calls == before {
		t.Error("a ticket change didn't re-resolve")
	}
}
