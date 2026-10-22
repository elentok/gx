package events

// Read shim for run-log lines written before the pause terminology. Only the
// log reader calls it; every writer uses the new words.
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
)

// ReadLegacy returns the type and kind an old log line's wire codes stand for
// today. New codes come back unchanged.
func ReadLegacy(t Type, k Kind) (Type, Kind) {
	if nt, ok := legacyTypes[t]; ok {
		t = nt
	}
	if nk, ok := legacyKinds[k]; ok {
		k = nk
	}
	return t, k
}
