package sidebar

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

// TestDeriveCollapsed_EveryEpicDefaultsCollapsed covers the default
// policy: open, closed and archived epics all start collapsed with no
// explicit state at all, so the sidebar first shows one row per epic.
func TestDeriveCollapsed_EveryEpicDefaultsCollapsed(t *testing.T) {
	t.Parallel()
	epics := []tickets.Epic{
		{Path: "closed-epic", Tickets: []tickets.Ticket{{Status: "done"}}},
		{Path: "open-epic", Tickets: []tickets.Ticket{{Status: "open"}}},
	}
	archived := []tickets.Epic{{Path: "archived-epic", Tickets: []tickets.Ticket{{Status: "done"}}}}

	got := DeriveCollapsed(nil, epics, archived, "")

	for _, path := range []string{"closed-epic", "open-epic", "archived-epic"} {
		if !got[path] {
			t.Fatalf("expected %s to default to collapsed, got %v", path, got[path])
		}
	}
}

// TestDeriveCollapsed_ExplicitToggleBeatsDefault covers both
// directions of an explicit user toggle overriding the declared default, for
// an open and a closed epic.
func TestDeriveCollapsed_ExplicitToggleBeatsDefault(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"open", "done"} {
		epics := []tickets.Epic{{Path: "epic", Tickets: []tickets.Ticket{{Status: status}}}}

		expanded := DeriveCollapsed(map[string]bool{"epic": false}, epics, nil, "")
		if expanded["epic"] {
			t.Fatalf("%s: expected explicit expand to beat the default, got collapsed=%v", status, expanded["epic"])
		}

		collapsed := DeriveCollapsed(map[string]bool{"epic": true}, epics, nil, "")
		if !collapsed["epic"] {
			t.Fatalf("%s: expected explicit collapse to be honored, got collapsed=%v", status, collapsed["epic"])
		}
	}
}

// TestDeriveCollapsed_SearchOverrideIsTransient covers search's
// auto-expand: it applies only while the epic still matches the live query,
// disappears once the query no longer matches (including an empty query),
// and never leaves a trace in the explicit map that was handed in.
func TestDeriveCollapsed_SearchOverrideIsTransient(t *testing.T) {
	t.Parallel()
	epics := []tickets.Epic{{Path: "closed-epic", Tickets: []tickets.Ticket{{Title: "widget", Status: "done"}}}}
	explicit := map[string]bool{}

	matched := DeriveCollapsed(explicit, epics, nil, "widget")
	if matched["closed-epic"] {
		t.Fatalf("expected search match to expand closed epic, got collapsed=%v", matched["closed-epic"])
	}
	if _, ok := explicit["closed-epic"]; ok {
		t.Fatalf("expected search override to leave no trace in the explicit map, got %v", explicit)
	}

	noQuery := DeriveCollapsed(explicit, epics, nil, "")
	if !noQuery["closed-epic"] {
		t.Fatalf("expected closed-epic default to reassert once search ends, got collapsed=%v", noQuery["closed-epic"])
	}

	noMatch := DeriveCollapsed(explicit, epics, nil, "gadget")
	if !noMatch["closed-epic"] {
		t.Fatalf("expected closed-epic default to reassert once it no longer matches, got collapsed=%v", noMatch["closed-epic"])
	}
}
