package notify

import "github.com/elentok/gx/notifykind"

// ChannelPolicy tells the router what a person chose per channel. The server
// answers from its live settings on every call, so a reload applies to the
// next send. A nil policy means every channel is on and every kind checked.
type ChannelPolicy interface {
	// ChannelOn reports whether the channel's switch is on. channel is the
	// transport name ("telegram", "slack").
	ChannelOn(channel string) bool
	// KindChecked reports whether the catalog id is in the channel's kind list.
	KindChecked(channel, kindID string) bool
}

// The router's order per channel: switch, then kind list, then the sink's own
// gates (mutes, budget, batch). The first "no" ends it, and a kind that is
// not checked never reaches the gate, so it leaves no trace in the state file.

func (s *chatEventSink) channelOff() bool {
	return s.policy != nil && !s.policy.ChannelOn(s.transport.name())
}

// silenced reports whether nothing may leave this sink now: rest period or switch off.
func (s *chatEventSink) silenced() bool { return s.isQuiet() || s.channelOff() }

// kindUnchecked reports whether msg is a catalog kind this channel's list lacks.
func (s *chatEventSink) kindUnchecked(msg *chatMessage) bool {
	return s.policy != nil && msg != nil && msg.catalog != "" && !s.policy.KindChecked(s.transport.name(), msg.catalog)
}

// Enabled reports whether any registered channel is on. Call sites that skip
// work when nothing would be sent ask this, not whether a destination exists.
func (c *ServerChat) Enabled() bool {
	for _, s := range c.globalSinks() {
		if !s.channelOff() {
			return true
		}
	}
	return false
}

// Active reports whether anything could send: a chat channel is on, or the
// server registered the Desktop or Push adapter, which decide for themselves.
// Call sites that skip work when nothing would be sent ask this.
func (c *ServerChat) Active() bool {
	return c != nil && (c.base.Desktop != nil || c.base.Push != nil || c.base.WebToast != nil || c.Enabled())
}

// as returns msg tagged with its catalog id.
func (m chatMessage) as(id string) chatMessage {
	m.catalog = id
	return m
}

// noticeCatalogID maps a ServerNotice kind to its catalog id. An unknown kind
// maps to "" and is treated as a system message.
func noticeCatalogID(kind string) string {
	switch kind {
	case "long-ticket":
		return notifykind.LongTicket
	case "extra-usage", "budget-soft-limit", "budget-hard-limit", "daily-summary":
		return notifykind.BudgetAndUsage
	case "disk-warn", "disk-resume", "disk-clean":
		return notifykind.Disk
	case "trunk-missing", "trunk-found", "project-unavailable":
		return notifykind.ProjectAndTrunk
	case "server-started":
		return notifykind.ServerStarted
	}
	return ""
}
