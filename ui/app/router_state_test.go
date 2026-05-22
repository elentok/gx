package app

import (
	"testing"

	"github.com/elentok/gx/ui/nav"
)

func TestRouterReplacePreservesPerTabHistory(t *testing.T) {
	r := newRouterState(nav.Route{Kind: nav.RouteLog, WorktreeRoot: "/repo"}, "/repo")
	r.push(nav.Route{Kind: nav.RouteCommit, WorktreeRoot: "/repo", Ref: "HEAD"})
	if len(r.history) != 1 {
		t.Fatalf("expected log history depth 1, got %d", len(r.history))
	}

	r.replace(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: "/repo"}, "/repo")
	if r.activeTab != nav.RouteStatus {
		t.Fatalf("expected status active tab, got %q", r.activeTab)
	}
	if len(r.history) != 0 {
		t.Fatalf("expected empty status history after replace, got %d", len(r.history))
	}

	r.replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: "/repo"}, "/repo")
	if r.activeTab != nav.RouteLog {
		t.Fatalf("expected log active tab, got %q", r.activeTab)
	}
	if len(r.history) != 1 {
		t.Fatalf("expected restored log history depth 1, got %d", len(r.history))
	}
	if r.history[0].Kind != nav.RouteCommit {
		t.Fatalf("expected restored route commit, got %q", r.history[0].Kind)
	}
}

func TestRouterCommitMapsToLogTab(t *testing.T) {
	r := newRouterState(nav.Route{Kind: nav.RouteCommit, WorktreeRoot: "/repo", Ref: "HEAD"}, "/repo")
	if r.activeTab != nav.RouteLog {
		t.Fatalf("expected commit to map to log tab, got %q", r.activeTab)
	}
	if len(r.history) != 1 || r.history[0].Kind != nav.RouteCommit {
		t.Fatalf("expected initial commit route on history stack")
	}
}

