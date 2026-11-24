package settings

import (
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/notifykind"
)

// NotifyChannel is one channel's switch and kind list, from config
// notification-channels.<channel>. The zero value is a new install: on, with
// the default list. That keeps servertest rigs that build Settings by hand
// sending as before.
type NotifyChannel struct {
	// Off is the switch turned off, inverted so a zero value keeps it on.
	Off bool
	// Kinds are the checked catalog ids. Nil means the default list; an empty
	// non-nil list means no kind.
	Kinds []string
}

// Enabled reports whether the channel's switch is on.
func (c NotifyChannel) Enabled() bool { return !c.Off }

// KindIDs returns the checked catalog ids, the default list when Kinds is nil.
func (c NotifyChannel) KindIDs() []string {
	if c.Kinds == nil {
		return notifykind.DefaultIDs()
	}
	return c.Kinds
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
	return NotifyChannel{Off: !c.Enabled, Kinds: c.Kinds}
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
