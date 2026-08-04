package config

// NotificationsConfig holds settings for external notification integrations.
type NotificationsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
}

// TelegramConfig holds Telegram bot credentials for sending notifications.
// An empty BotToken is the "off" state.
type TelegramConfig struct {
	BotToken string `json:"bot-token"`
	ChatID   string `json:"chat-id"`
}

// DefaultNotificationsConfig returns the notifications defaults.
func DefaultNotificationsConfig() NotificationsConfig {
	return NotificationsConfig{
		Telegram: TelegramConfig{
			BotToken: "",
			ChatID:   "",
		},
	}
}
