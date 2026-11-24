package server

import (
	"context"
	"time"

	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/webpush"
	"github.com/elentok/gx/server/wire"
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
	if !channelAllows(d.s, notify.ChannelDesktop, n.Kind, n.System) {
		return
	}
	cur := d.s.settings.Load()
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
	banner := !n.Session || cur.AgentsBanners
	sound := n.Session && n.Sound != "" && cur.AgentsSounds.Enabled
	if !(banner || sound) || d.s.rest.Quiet() {
		return
	}
	// The banner and its sound are one send, and the cap is the last gate.
	send, trip := channelCap(d.s, notify.ChannelDesktop)
	if trip {
		d.notify(osnotify.Notification{Title: "gx: Desktop paused", Body: breakerNotice("Desktop"), Group: "gx-desktop-breaker"})
	}
	if !send {
		return
	}
	if banner {
		d.notify(note)
	}
	if sound {
		d.s.cfg.Player.Play(context.Background(), cur.AgentsSounds, n.Sound)
	}
}

func (d *desktopAdapter) notify(note osnotify.Notification) {
	if err := d.s.cfg.Notifier.Notify(context.Background(), note); err != nil {
		d.s.log.Warn("desktop banner", "title", note.Title, "error", err)
	}
}

func (d *desktopAdapter) Preflight(kind string, system bool) string {
	return preflight(d.s, notify.ChannelDesktop, kind, system)
}

func (w *webToastAdapter) Preflight(kind string, system bool) string {
	return preflight(w.s, notify.ChannelWebToast, kind, system)
}

// channelAllows is the adapters' gate: switch and kind list through
// NotifyChannel.Allows, and false for a nil Server or nil settings.
func channelAllows(s *Server, name, kind string, system bool) bool {
	if s == nil {
		return false
	}
	ch, ok := channelFrom(&s.settings, name)
	return ok && ch.Allows(kind, system)
}

// preflight is what the channel would do with a note of kind now, in the router's
// order: switch, kind list, breaker. It counts no attempt.
func preflight(s *Server, name, kind string, system bool) string {
	if s == nil {
		return notify.OutcomeUnconfigured
	}
	ch, ok := channelFrom(&s.settings, name)
	if !ok {
		return notify.OutcomeUnconfigured
	}
	switch {
	case !ch.Enabled():
		return notify.OutcomeOff
	case !ch.Allows(kind, system):
		return notify.OutcomeUnchecked
	}
	if state, err := notify.LoadNotificationState(s.cfg.Chat.GateStatePath); err == nil && state.Transports[name].ChannelTripped(time.Now(), s.cfg.ChannelBreakerClear) {
		return notify.OutcomeTripped
	}
	return notify.OutcomeSent
}

// channelCap counts one attempt on a channel against the shared state file.
func channelCap(s *Server, channel string) (send, trip bool) {
	return notify.ChannelCap(s.cfg.Chat.GateStatePath, channel, time.Now(), s.cfg.ChannelBreakerClear)
}

func breakerNotice(channel string) string {
	return channel + " sent too many notifications in a minute, so gx stops it until it has been quiet for 5 minutes."
}

// webToastAdapter is the Web toast channel of the router: the switch and the
// kind list apply here. Nothing is stored, so a page that is not open misses
// the toast. s is set once the Server exists.
type webToastAdapter struct{ s *Server }

func (w *webToastAdapter) Show(n notify.WebToastNote) {
	if w.s == nil {
		return
	}
	if !channelAllows(w.s, notify.ChannelWebToast, n.Kind, n.System) {
		return
	}
	send, trip := channelCap(w.s, notify.ChannelWebToast)
	if trip {
		w.s.events.PublishEvent(wire.Event{Type: wire.EventNotificationToast, Title: "gx: Web toast paused", Body: breakerNotice("Web toast"), Path: "/"})
	}
	if !send {
		return
	}
	w.s.events.PublishEvent(wire.Event{Type: wire.EventNotificationToast, Title: n.Title, Body: n.Body, Path: n.Target.Path()})
}
