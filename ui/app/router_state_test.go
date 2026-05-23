package app

import (
	"testing"

	"github.com/elentok/gx/ui/nav"
)

func TestRouterSwitchPreservesPerTabHistory(t *testing.T) {
	r := newRouterState(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: "/repo"}, "/repo")
	r.push(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: "/repo", Ref: "HEAD"})
	if len(r.history) != 1 {
		t.Fatalf("expected log history depth 1, got %d", len(r.history))
	}

	r.replace(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: "/repo"}, "/repo")
	if r.activeTab != nav.TabStatus {
		t.Fatalf("expected status active tab, got %q", r.activeTab)
	}
	if len(r.history) != 0 {
		t.Fatalf("expected empty status history after replace, got %d", len(r.history))
	}

	r.replace(nav.ViewState{Tab: nav.TabLog, WorktreeRoot: "/repo"}, "/repo")
	if r.activeTab != nav.TabLog {
		t.Fatalf("expected log active tab, got %q", r.activeTab)
	}
	if len(r.history) != 1 {
		t.Fatalf("expected restored log history depth 1, got %d", len(r.history))
	}
	if r.history[0].Tab != nav.TabCommit {
		t.Fatalf("expected restored route commit, got %q", r.history[0].Tab)
	}
}

func TestRouterCommitMapsToLogTab(t *testing.T) {
	r := newRouterState(nav.ViewState{Tab: nav.TabCommit, WorktreeRoot: "/repo", Ref: "HEAD"}, "/repo")
	if r.activeTab != nav.TabLog {
		t.Fatalf("expected commit to map to log tab, got %q", r.activeTab)
	}
	if len(r.history) != 1 || r.history[0].Tab != nav.TabCommit {
		t.Fatalf("expected initial commit route on history stack")
	}
}
