package notify

import "github.com/elentok/gx/notifykind"

// PushNote is one notification handed to the Push channel. The adapter applies
// the channel switch and kind list; callers send every one.
type PushNote struct {
	Kind        string // catalog id
	Title, Body string
	Target      Target
	// Tag replaces events with the same tag inside the merge delay. Empty uses
	// Target.Tag(Kind).
	Tag    string
	Urgent bool
	// Merged marks an event the caller already merged (the session banner), so
	// the dispatcher sends it without holding it again.
	Merged bool
	// System skips the channel's kind list; the switch and the rest still apply.
	System bool
}

// Push is the channel that sends Web Push to the subscribed devices.
type Push interface{ Send(PushNote) }

// Push hands one note to the Push channel. A nil chat or channel sends none.
func (c *ServerChat) Push(n PushNote) {
	if c == nil || c.base.Push == nil {
		return
	}
	c.base.Push.Send(n)
}

// Tag is the merge tag of a kind aimed at the target. A pause keeps the
// ticket or epic tag the web app already knows; other kinds are prefixed with
// their id so two kinds on one target do not replace each other.
func (t Target) Tag(kind string) string {
	var key string
	switch {
	case t.Ticket != "":
		key = "ticket:" + t.Ticket
	case t.Epic != "":
		key = "epic:" + t.Epic
	case t.Design != "":
		key = "design:" + t.Design
	case t.Session != "":
		key = "session:" + t.Session
	}
	switch kind {
	case notifykind.PausedTicket, notifykind.EpicPaused:
		if key != "" {
			return key
		}
	}
	if key == "" {
		return "notice:" + kind
	}
	return kind + ":" + key
}
