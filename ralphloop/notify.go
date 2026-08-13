package ralphloop

import (
	"fmt"
	"time"

	"github.com/elentok/gx/config"
)

// notifyKindCLI tags gate/run-log bookkeeping for gx notify's synchronous
// send path — the only kind of event this path ever produces.
const notifyKindCLI = "cli-notify"

// SendMessage sends text to every notification service configured in cfg
// (Telegram and/or Slack), no-op'ing per-service when that service's
// credentials are absent — the same configured-check `gx config
// test-notifications` uses. It returns the names of the services it actually
// sent to ("telegram", "slack") and an error joining any per-service
// failures, so a Slack success is still visible even if Telegram fails.
func SendMessage(cfg config.NotificationsConfig, text string) ([]string, error) {
	return sendMessage(cfg, text, telegramAPIBaseURL)
}

// sendMessage is SendMessage with the Telegram API base URL injectable, so
// tests can point it at a fake server instead of the real Telegram API.
func sendMessage(cfg config.NotificationsConfig, text, telegramBaseURL string) ([]string, error) {
	var sent []string
	var errs []error

	if cfg.Telegram.BotToken != "" {
		allowed, err := gateCLISend("telegram")
		if err != nil {
			errs = append(errs, fmt.Errorf("telegram: %w", err))
		} else if allowed {
			if err := sendTelegramMessage(cfg.Telegram.BotToken, cfg.Telegram.ChatID, telegramBaseURL, text); err != nil {
				errs = append(errs, fmt.Errorf("telegram: %w", err))
			} else {
				sent = append(sent, "telegram")
			}
		}
	}

	if cfg.Slack.WebhookURL != "" {
		allowed, err := gateCLISend("slack")
		if err != nil {
			errs = append(errs, fmt.Errorf("slack: %w", err))
		} else if allowed {
			if err := SendSlackMessage(cfg.Slack.WebhookURL, text); err != nil {
				errs = append(errs, fmt.Errorf("slack: %w", err))
			} else {
				sent = append(sent, "slack")
			}
		}
	}

	if len(errs) == 0 {
		return sent, nil
	}
	joined := errs[0]
	for _, err := range errs[1:] {
		joined = fmt.Errorf("%w; %w", joined, err)
	}
	return sent, joined
}

// SendTestBatch sends a fixed 2-message test batch to every notification
// service configured in cfg, through the exact same renderBatch join a real
// flush uses (see SendTelegramTestBatch/SendSlackTestBatch) — `gx notify
// --test-batch`'s entry point for reproducing/verifying the batch-separator
// escaping bug against the real APIs, no-op'ing per-service when that
// service's credentials are absent, same shape as SendMessage.
func SendTestBatch(cfg config.NotificationsConfig) ([]string, error) {
	var sent []string
	var errs []error

	if cfg.Telegram.BotToken != "" {
		if err := SendTelegramTestBatch(cfg.Telegram.BotToken, cfg.Telegram.ChatID); err != nil {
			errs = append(errs, fmt.Errorf("telegram: %w", err))
		} else {
			sent = append(sent, "telegram")
		}
	}

	if cfg.Slack.WebhookURL != "" {
		if err := SendSlackTestBatch(cfg.Slack.WebhookURL); err != nil {
			errs = append(errs, fmt.Errorf("slack: %w", err))
		} else {
			sent = append(sent, "slack")
		}
	}

	if len(errs) == 0 {
		return sent, nil
	}
	joined := errs[0]
	for _, err := range errs[1:] {
		joined = fmt.Errorf("%w; %w", joined, err)
	}
	return sent, joined
}

// gateCLISend runs transport's send through the notification gate with
// source "cli" — there's no ticket to park a trip against, so parkTicket is
// nil. A trip still writes its bookkeeping; the caller just skips sending.
func gateCLISend(transport string) (bool, error) {
	result, err := NotificationGate(transport, notifyKindCLI, "cli", time.Now(), true, nil)
	if err != nil {
		return false, err
	}
	return result.Decision == Allowed, nil
}
