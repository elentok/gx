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

// The outcome strings are a public contract: the notify routes, the CLI and the
// web UI show them.
const (
	OutcomeSent         = "sent"
	OutcomeOff          = "off"          // the channel's switch is off
	OutcomeUnchecked    = "unchecked"    // the kind is not in the channel's list
	OutcomeMuted        = "muted"        // a rest period or a manual mute
	OutcomeTripped      = "tripped"      // the send-rate breaker is tripped
	OutcomeUnconfigured = "unconfigured" // nothing to send to
)

// The channel names, in the order the Notifications settings list them.
const (
	ChannelDesktop  = "desktop"
	ChannelWebToast = "web-toast"
	ChannelTelegram = "telegram"
	ChannelSlack    = "slack"
	ChannelPush     = "push"
)

// ChannelOutcome is what one channel did with one message.
type ChannelOutcome struct{ Channel, Outcome string }

// Preflighter is implemented by the Desktop, Web toast and Push adapters: it
// says what the channel would do with a note of kind now. system skips the kind list.
type Preflighter interface {
	Preflight(kind string, system bool) string
}

func adapterOutcome(channel any, kind string, system bool) string {
	if channel == nil {
		return OutcomeUnconfigured
	}
	if p, ok := channel.(Preflighter); ok {
		return p.Preflight(kind, system)
	}
	return OutcomeSent
}

// Route sends msg to all five channels and returns each one's outcome. A system
// message skips the kind lists; every other gate applies. The Desktop, Web toast
// and Push outcomes are read just before the send, so a racing send can differ.
func (c *ServerChat) Route(msg chatMessage, target Target, system bool) []ChannelOutcome {
	if c == nil {
		return nil
	}
	kind := notifykind.CLIMessage
	if !system && msg.catalog != "" {
		kind = msg.catalog
	}
	byChannel := map[string]string{
		ChannelDesktop:  adapterOutcome(c.base.Desktop, kind, system),
		ChannelWebToast: adapterOutcome(c.base.WebToast, kind, system),
		ChannelPush:     adapterOutcome(c.base.Push, kind, system),
		ChannelTelegram: OutcomeUnconfigured,
		ChannelSlack:    OutcomeUnconfigured,
	}
	c.fanOut(msg, target, false, system)
	for _, s := range c.globalSinks() {
		byChannel[s.transport.name()] = s.sendItem(newBatchedMessage(s.style, msg), "cli", "")
	}
	var out []ChannelOutcome
	for _, ch := range []string{ChannelDesktop, ChannelWebToast, ChannelTelegram, ChannelSlack, ChannelPush} {
		out = append(out, ChannelOutcome{Channel: ch, Outcome: byChannel[ch]})
	}
	return out
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
