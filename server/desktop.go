package server

import (
	"context"
	"slices"

	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/webpush"
)

// desktopAdapter is the Desktop channel of the router. The switch is a
// decorator of the server's notifier and player (so system banners obey it
// too); the adapter applies the kind list and the Agents settings. It reads
// the live settings on every call, so a reload applies to the next send. s is
// set once the Server exists.
type desktopAdapter struct {
	s *Server
	// webURL is the web listener's base URL, "" when it is off.
	webURL func() string
}

func (d *desktopAdapter) Show(n notify.DesktopNote) {
	if d.s == nil {
		return
	}
	cur := d.s.settings.Load()
	if cur == nil || !cur.NotifyChannels.Desktop.Enabled() || !slices.Contains(cur.NotifyChannels.Desktop.KindIDs(), n.Kind) {
		return
	}
	note := osnotify.Notification{Title: n.Title, Body: n.Body, Group: n.Group, Activate: n.Activate, Execute: n.Execute, Link: n.Link}
	if !n.Session {
		path := n.Target.Path()
		note.Body = webpush.CutBody(n.Body, webpush.BodyChars)
		note.Group = "gx-" + n.Kind + ":" + path
		if base := d.webURL(); base != "" {
			note.Link = base + "/#" + path
		}
	}
	// The Agents banner setting applies to session banners only.
	if !n.Session || cur.AgentsBanners {
		if err := d.s.cfg.Notifier.Notify(context.Background(), note); err != nil {
			d.s.log.Warn("desktop banner", "kind", n.Kind, "error", err)
		}
	}
	if n.Session && n.Sound != "" && cur.AgentsSounds.Enabled {
		d.s.cfg.Player.Play(context.Background(), cur.AgentsSounds, n.Sound)
	}
}
