package server

import (
	"slices"

	"github.com/elentok/gx/server/settings"
)

// chatPolicy answers the router from the live settings on every call, so a
// reload applies to the next send. It is a pointer in ServerChatConfig so the
// config stays comparable; h is set once the Server exists.
type chatPolicy struct{ h *settings.Holder }

func (p *chatPolicy) channel(name string) (settings.NotifyChannel, bool) {
	if p.h == nil {
		return settings.NotifyChannel{}, false
	}
	cur := p.h.Load()
	if cur == nil {
		return settings.NotifyChannel{}, false
	}
	switch name {
	case "telegram":
		return cur.NotifyChannels.Telegram, true
	case "slack":
		return cur.NotifyChannels.Slack, true
	}
	return settings.NotifyChannel{}, false
}

func (p *chatPolicy) ChannelOn(name string) bool {
	c, ok := p.channel(name)
	return !ok || c.Enabled()
}

func (p *chatPolicy) KindChecked(name, id string) bool {
	c, ok := p.channel(name)
	return !ok || slices.Contains(c.KindIDs(), id)
}
