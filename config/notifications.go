package config

import (
	"fmt"

	"github.com/elentok/gx/notifykind"
)

// NotificationsConfig holds settings for external notification integrations.
type NotificationsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
	Slack    SlackConfig    `json:"slack"`
}

// TelegramConfig holds Telegram bot credentials for sending notifications.
// An empty BotToken is the "off" state.
type TelegramConfig struct {
	BotToken string `json:"bot-token" secret:"true"`
	ChatID   string `json:"chat-id" secret:"true"`
}

// SlackConfig holds a Slack workflow webhook URL for sending notifications —
// a webhook that accepts a single {"text": "..."} body. An empty WebhookURL
// is the "off" state.
type SlackConfig struct {
	WebhookURL string `json:"webhook-url" secret:"true"`
}

// ChannelConfig is one channel's switch and checked kind ids. Kinds is a list
// of notifykind ids; an explicit empty list is valid and means no kind.
type ChannelConfig struct {
	Enabled bool     `json:"enabled"`
	Kinds   []string `json:"kinds"`
}

// NotificationChannelsConfig holds the five channels. It is a top-level block
// so a project's notifications override never replaces it.
type NotificationChannelsConfig struct {
	Desktop  ChannelConfig `json:"desktop"`
	WebToast ChannelConfig `json:"web-toast"`
	Telegram ChannelConfig `json:"telegram"`
	Slack    ChannelConfig `json:"slack"`
	Push     ChannelConfig `json:"push"`
}

// rawChannel is one channel as written in the file.
type rawChannel struct {
	Enabled *bool    `json:"enabled"`
	Kinds   []string `json:"kinds"`
}

type rawChannels struct {
	Desktop  *rawChannel `json:"desktop"`
	WebToast *rawChannel `json:"web-toast"`
	Telegram *rawChannel `json:"telegram"`
	Slack    *rawChannel `json:"slack"`
	Push     *rawChannel `json:"push"`
}

// DefaultNotificationChannelsConfig has every channel on with the default
// kind list (every kind except Long ticket).
func DefaultNotificationChannelsConfig() NotificationChannelsConfig {
	ch := func() ChannelConfig {
		return ChannelConfig{Enabled: true, Kinds: notifykind.DefaultIDs()}
	}
	return NotificationChannelsConfig{Desktop: ch(), WebToast: ch(), Telegram: ch(), Slack: ch(), Push: ch()}
}

// apply merges the file's block over c. A list with an id outside the catalog
// is refused, naming the key and the id.
func (raw *rawChannels) apply(c *NotificationChannelsConfig) error {
	if raw == nil {
		return nil
	}
	for _, p := range []struct {
		name string
		raw  *rawChannel
		dst  *ChannelConfig
	}{
		{"desktop", raw.Desktop, &c.Desktop},
		{"web-toast", raw.WebToast, &c.WebToast},
		{"telegram", raw.Telegram, &c.Telegram},
		{"slack", raw.Slack, &c.Slack},
		{"push", raw.Push, &c.Push},
	} {
		if p.raw == nil {
			continue
		}
		if p.raw.Enabled != nil {
			p.dst.Enabled = *p.raw.Enabled
		}
		if p.raw.Kinds != nil {
			for _, id := range p.raw.Kinds {
				if !notifykind.Valid(id) {
					return fmt.Errorf("notification-channels.%s.kinds: %q is not a notification kind", p.name, id)
				}
			}
			p.dst.Kinds = p.raw.Kinds
		}
	}
	return nil
}

// DefaultNotificationsConfig returns the notifications defaults.
func DefaultNotificationsConfig() NotificationsConfig {
	return NotificationsConfig{
		Telegram: TelegramConfig{
			BotToken: "",
			ChatID:   "",
		},
		Slack: SlackConfig{
			WebhookURL: "",
		},
	}
}
