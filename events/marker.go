package events

import "strings"

// Marker is a stable tag a pause source owns and the chat wording matches on,
// so wording never depends on error text. It travels as a prefix of the pause
// reason, because Pause(kind, reason) carries only those two. A source declares
// `const MyMarker events.Marker = "my-marker"` and builds the reason with
// MyMarker.Reason(detail); the wording table in server/ticketwrite maps
// (kind, marker) to a sentence. Detail after the marker is free text.
type Marker string

// Markers for iteration-error pauses whose cause a typed error identifies.
const (
	StallNudgeMarker  Marker = "stall-nudge"
	SessionDiedMarker Marker = "session-died"
	HerdrLostMarker   Marker = "herdr-agent-lost"
	WorktreeExists    Marker = "worktree-exists"
)

// markerSep ends the marker; the detail follows it.
const markerSep = ": "

// Reason is the pause reason for this marker with free-text detail after it.
func (m Marker) Reason(detail string) string {
	if detail == "" {
		return string(m)
	}
	return string(m) + markerSep + detail
}

// In reports whether reason carries this marker as its prefix.
func (m Marker) In(reason string) bool {
	return reason == string(m) || strings.HasPrefix(reason, string(m)+markerSep)
}
