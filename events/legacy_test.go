package events

import "testing"

func TestLegacyKindRoundTrip(t *testing.T) {
	for old, nu := range legacyKinds {
		if got := CurrentKind(old); got != nu {
			t.Errorf("CurrentKind(%q) = %q, want %q", old, got, nu)
		}
		if got := LegacyKind(nu); got != old {
			t.Errorf("LegacyKind(%q) = %q, want %q", nu, got, old)
		}
	}
	if got := LegacyKind(ZeroCommit); got != ZeroCommit {
		t.Errorf("LegacyKind(%q) = %q, want it unchanged", ZeroCommit, got)
	}
}

func TestCurrentLogType(t *testing.T) {
	if got := CurrentLogType("park.question"); got != "pause.question" {
		t.Errorf("CurrentLogType(park.question) = %q", got)
	}
	if got := CurrentLogType("pause.answer"); got != "pause.answer" {
		t.Errorf("CurrentLogType(pause.answer) = %q", got)
	}
}

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
