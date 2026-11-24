package notify

import (
	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/sound"
)

// Target is what a notification is about. The first non-empty field wins; the
// zero value is the app home.
type Target struct {
	Ticket  string // "project:epic/id"
	Epic    string // "project:epic"
	Design  string // the Design's root, as the Design page route names it
	Session string // a session id
}

// Path is the web route of the target, the one Push and Desktop both send.
func (t Target) Path() string {
	switch {
	case t.Ticket != "":
		return "/t/" + t.Ticket
	case t.Epic != "":
		return "/e/" + t.Epic
	case t.Design != "":
		return "/d/" + t.Design
	case t.Session != "":
		return "/agents/" + t.Session
	}
	return "/"
}

// DesktopNote is one notification handed to the Desktop channel. The adapter
// applies the channel's kind list and settings, so callers send every one.
type DesktopNote struct {
	Kind          string // catalog id
	Title, Body   string
	Target        Target
	Sound         sound.Name // empty plays none
	Session       bool       // the caller sets Group, Activate, Execute and Link itself
	Group         string
	Activate      string
	Execute, Link string
	// System skips the channel's kind list; the switch and the rest still apply.
	System bool
}

// Desktop is the channel that shows OS banners and plays sounds.
type Desktop interface{ Show(DesktopNote) }

// Desktop hands one note to the Desktop channel. A nil chat or channel sends none.
func (c *ServerChat) Desktop(n DesktopNote) {
	if c == nil || c.base.Desktop == nil {
		return
	}
	c.base.Desktop.Show(n)
}

// show hands a catalog message to Desktop as a banner without sound. A system
// message has no catalog id and is never a kind.
// It hands the same words to Push, once, whatever the project's override picks.
func (c *ServerChat) show(msg chatMessage, target Target) { c.showUrgent(msg, target, false) }

func (c *ServerChat) showUrgent(msg chatMessage, target Target, urgent bool) {
	c.fanOut(msg, target, urgent, false)
}

// fanOut hands msg to Desktop, Push and Web toast. A message with no catalog id
// is never a kind, so only a system message (which skips the kind lists) may go
// out without one.
func (c *ServerChat) fanOut(msg chatMessage, target Target, urgent, system bool) {
	if msg.catalog == "" && !system {
		return
	}
	kind := msg.catalog
	if system {
		kind = notifykind.CLIMessage
	}
	title, body := pushText(msg)
	c.Desktop(DesktopNote{Kind: kind, Title: title, Body: body, Target: target, System: system})
	c.Push(PushNote{Kind: kind, Title: title, Body: body, Target: target, Urgent: urgent, System: system})
	c.WebToast(WebToastNote{Kind: kind, Title: title, Body: body, Target: target, System: system})
}

func ticketTarget(project, epic, identifier string) Target {
	if identifier == "" {
		return Target{Epic: project + ":" + epic}
	}
	return Target{Ticket: project + ":" + epic + "/" + identifier}
}
