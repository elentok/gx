package settings

import (
	"slices"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop/notify"
)

// NotifyChannel is one channel's switch and kind list, from config
// notification-channels.<channel>.
type NotifyChannel struct {
	// Enabled is the channel's switch.
	Enabled bool
	// Kinds are the checked catalog ids; an empty list means no kind.
	Kinds []string
}

// KindIDs returns the checked catalog ids as stored.
func (c NotifyChannel) KindIDs() []string { return c.Kinds }

// Allows is the one gate: the switch is on, and the kind is checked or the
// note is a system one.
func (c NotifyChannel) Allows(kind string, system bool) bool {
	return c.Enabled && (system || slices.Contains(c.KindIDs(), kind))
}

// ByID returns the channel with the notify.Channel* id.
func (n NotifyChannels) ByID(id string) (NotifyChannel, bool) {
	switch id {
	case notify.ChannelDesktop:
		return n.Desktop, true
	case notify.ChannelWebToast:
		return n.WebToast, true
	case notify.ChannelTelegram:
		return n.Telegram, true
	case notify.ChannelSlack:
		return n.Slack, true
	case notify.ChannelPush:
		return n.Push, true
	}
	return NotifyChannel{}, false
}

// NotifyChannels holds the five channels' settings.
type NotifyChannels struct {
	Desktop  NotifyChannel
	WebToast NotifyChannel
	Telegram NotifyChannel
	Slack    NotifyChannel
	Push     NotifyChannel
}

func notifyChannelFrom(c config.ChannelConfig) NotifyChannel {
	return NotifyChannel{Enabled: c.Enabled, Kinds: c.Kinds}
}

func notifyChannelsFrom(c config.NotificationChannelsConfig) NotifyChannels {
	return NotifyChannels{
		Desktop:  notifyChannelFrom(c.Desktop),
		WebToast: notifyChannelFrom(c.WebToast),
		Telegram: notifyChannelFrom(c.Telegram),
		Slack:    notifyChannelFrom(c.Slack),
		Push:     notifyChannelFrom(c.Push),
	}
}
