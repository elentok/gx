package events

// The one old-to-new table for the pause terminology. The read shims (the run
// log reader, the ticket log reader and the ticket schema) all call it; every
// writer uses the new words. It lives here because events is the lowest
// package in the import graph.
var (
	legacyTypes = map[Type]Type{
		"needs-answer":  Paused,
		"needs-repair":  Paused,
		"land-deferred": MergeDeferred,
		"manual-land":   ManualMerge,
		"ticket-reset":  TicketStartedOver,
	}
	legacyKinds = map[Kind]Kind{
		"manual-park":    Manual,
		"all-parked":     AllPaused,
		"ambiguous-land": AmbiguousMerge,
		"land-conflict":  MergeConflict,
		"land-error":     MergeError,
	}
	// legacyLogTypes are the event types of a ticket's own log (not the run
	// log), spelled as the tickets package spells them today.
	legacyLogTypes = map[string]string{
		"park.question":   "pause.question",
		"park.answer":     "pause.answer",
		"park.permission": "pause.permission",
		"park.closed":     "pause.closed",
	}
	oldKinds = reverse(legacyKinds)
)

func reverse[K, V comparable](m map[K]V) map[V]K {
	out := make(map[V]K, len(m))
	for k, v := range m {
		out[v] = k
	}
	return out
}

// ReadLegacy returns the type and kind an old log line's wire codes stand for
// today. New codes come back unchanged.
func ReadLegacy(t Type, k Kind) (Type, Kind) {
	if nt, ok := legacyTypes[t]; ok {
		t = nt
	}
	return t, CurrentKind(k)
}

// CurrentKind is the kind an old spelling stands for today. A current or
// unknown kind comes back unchanged.
func CurrentKind(k Kind) Kind {
	if nk, ok := legacyKinds[k]; ok {
		return nk
	}
	return k
}

// LegacyKind is k spelled the old way, or k itself when it was never renamed.
func LegacyKind(k Kind) Kind {
	if old, ok := oldKinds[k]; ok {
		return old
	}
	return k
}

// CurrentLogType is the ticket-log event type an old spelling stands for today.
// A current or unknown type comes back unchanged.
func CurrentLogType(t string) string {
	if nt, ok := legacyLogTypes[t]; ok {
		return nt
	}
	return t
}
