package config

// NotificationsConfig holds settings for external notification integrations.
type NotificationsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
	Slack    SlackConfig    `json:"slack"`
}

// TelegramConfig holds Telegram bot credentials for sending notifications.
// An empty BotToken is the "off" state.
type TelegramConfig struct {
	BotToken string `json:"bot-token"`
	ChatID   string `json:"chat-id"`
}

// SlackConfig holds a Slack workflow webhook URL for sending notifications —
// a webhook that accepts a single {"text": "..."} body. An empty WebhookURL
// is the "off" state.
type SlackConfig struct {
	WebhookURL string `json:"webhook-url"`
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
