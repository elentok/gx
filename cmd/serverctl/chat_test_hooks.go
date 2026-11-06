package serverctl

import (
	"fmt"
	"time"

	"github.com/elentok/gx/ralphloop/notify"
)

// chatTestHooks reads the Chat test hooks gxfixture sets on a separate server:
// the batch tick, the Progress interval and the Telegram host. Unset means
// zero, so the default holds.
func chatTestHooks(getenv func(string) string) (notify.ServerChatConfig, error) {
	var cfg notify.ServerChatConfig
	for _, d := range []struct {
		name string
		dst  *time.Duration
	}{
		{"GX_CHAT_FLUSH_INTERVAL", &cfg.FlushInterval},
		{"GX_CHAT_PROGRESS_INTERVAL", &cfg.ProgressInterval},
	} {
		if v := getenv(d.name); v != "" {
			dur, err := time.ParseDuration(v)
			if err != nil {
				return cfg, fmt.Errorf("%s: %w", d.name, err)
			}
			*d.dst = dur
		}
	}
	cfg.TelegramBaseURL = getenv("GX_CHAT_TELEGRAM_BASE_URL")
	return cfg, nil
}
