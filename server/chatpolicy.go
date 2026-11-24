package server

import (
	"slices"

	"github.com/elentok/gx/server/settings"
)

// channelFrom is the one nil-settings rule: no loaded settings, or an id that is
// not a channel, gives no channel, and every caller then blocks the send.
func channelFrom(h *settings.Holder, id string) (settings.NotifyChannel, bool) {
	if h == nil {
		return settings.NotifyChannel{}, false
	}
	cur := h.Load()
	if cur == nil {
		return settings.NotifyChannel{}, false
	}
	return cur.NotifyChannels.ByID(id)
}

// chatPolicy answers the router from the live settings on every call, so a
// reload applies to the next send. It is a pointer in ServerChatConfig so the
// config stays comparable; h is set once the Server exists.
type chatPolicy struct{ h *settings.Holder }

func (p *chatPolicy) ChannelOn(name string) bool {
	c, ok := channelFrom(p.h, name)
	return ok && c.Enabled
}

func (p *chatPolicy) KindChecked(name, id string) bool {
	c, ok := channelFrom(p.h, name)
	return ok && slices.Contains(c.KindIDs(), id)
}
