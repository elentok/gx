package events

import "testing"

func TestReadLegacy(t *testing.T) {
	cases := []struct {
		inType Type
		inKind Kind
		typ    Type
		kind   Kind
	}{
		{"needs-answer", ZeroCommit, Paused, ZeroCommit},
		{"needs-repair", "manual-park", Paused, Manual},
		{"needs-repair", "ambiguous-land", Paused, AmbiguousMerge},
		{"needs-repair", "land-conflict", Paused, MergeConflict},
		{"needs-repair", "land-error", Paused, MergeError},
		{"land-deferred", "", MergeDeferred, ""},
		{"manual-land", "", ManualMerge, ""},
		{"ticket-reset", "", TicketStartedOver, ""},
		{Deadlocked, "all-parked", Deadlocked, AllPaused},
		{Paused, Manual, Paused, Manual},
	}
	for _, c := range cases {
		typ, kind := ReadLegacy(c.inType, c.inKind)
		if typ != c.typ || kind != c.kind {
			t.Errorf("ReadLegacy(%q, %q) = (%q, %q), want (%q, %q)", c.inType, c.inKind, typ, kind, c.typ, c.kind)
		}
		if kind != "" && !kind.Valid() {
			t.Errorf("ReadLegacy(%q, %q) kind %q is not in the enum", c.inType, c.inKind, kind)
		}
	}
}
