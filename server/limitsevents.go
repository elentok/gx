package server

import "github.com/elentok/gx/server/wire"

// limitsVisiblyChanged reports whether a window result changes what the menu
// shows: a new or changed window, or a stale one a report just confirmed.
func limitsVisiblyChanged(r *wire.LimitResult, now int64) bool {
	if r == nil {
		return false
	}
	switch r.Outcome {
	case wire.LimitNew, wire.LimitChanged:
		return true
	case wire.LimitConfirmed:
		return r.PrevReceivedAt > 0 && now-r.PrevReceivedAt > wire.LimitsStaleAfterSeconds
	}
	return false
}
