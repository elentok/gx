package notify

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/notifykind"
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
func SendMessage(cfg config.NotificationsConfig, statePath, text string) ([]string, error) {
	return sendMessage(cfg, statePath, text, cliTelegramBaseURL())
}

// sendMessage is SendMessage with the Telegram API base URL injectable, so
// tests can point it at a fake server instead of the real Telegram API.
func sendMessage(cfg config.NotificationsConfig, statePath, text, telegramBaseURL string) ([]string, error) {
	out := []ChannelOutcome{{Channel: ChannelTelegram}, {Channel: ChannelSlack}}
	out, err := sendOutcomes(cfg, out, statePath, text, telegramBaseURL)
	var sent []string
	for _, o := range out {
		if o.Outcome == OutcomeSent {
			sent = append(sent, o.Channel)
		}
	}
	return sent, err
}

// sendOutcomes sends text to each chat channel in out whose Outcome is still
// empty and fills that Outcome in: unconfigured, muted by the gate, failed or
// sent. A channel with an Outcome already (from ChatGate) is left alone. The
// error joins the per-channel failures.
func sendOutcomes(cfg config.NotificationsConfig, out []ChannelOutcome, statePath, text, telegramBaseURL string) ([]ChannelOutcome, error) {
	var errs []error
	sends := []struct {
		configured bool
		send       func() error
	}{
		{cfg.Telegram.BotToken != "", func() error {
			return sendTelegramMessage(cfg.Telegram.BotToken, cfg.Telegram.ChatID, telegramBaseURL, text)
		}},
		{cfg.Slack.WebhookURL != "", func() error { return SendSlackMessage(cfg.Slack.WebhookURL, text) }},
	}
	for i, s := range sends {
		o := &out[i]
		if o.Outcome != "" {
			continue
		}
		if !s.configured {
			o.Outcome = OutcomeUnconfigured
			continue
		}
		allowed, err := gateCLISend(statePath, o.Channel)
		if err == nil && allowed {
			err = s.send()
		}
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", o.Channel, err))
			o.Outcome = OutcomeFailed
		case allowed:
			o.Outcome = OutcomeSent
		default:
			o.Outcome = OutcomeMuted
		}
	}
	return out, errors.Join(errs...)
}

// OutcomeFailed is the direct fallback's outcome for a send that errored. The
// server never answers it: a failed send is retried there, not reported.
const OutcomeFailed = "failed"

// SendCLIMessage is SendMessage for the CLI without a server: it applies each
// chat channel's switch and the CLI message kind in channels first, as the
// router does, and says what each of Telegram and Slack did, in that order.
func SendCLIMessage(cfg config.NotificationsConfig, channels config.NotificationChannelsConfig, statePath, text string) ([]ChannelOutcome, error) {
	return sendCLIMessage(cfg, channels, statePath, text, cliTelegramBaseURL())
}

func sendCLIMessage(cfg config.NotificationsConfig, channels config.NotificationChannelsConfig, statePath, text, telegramBaseURL string) ([]ChannelOutcome, error) {
	return sendOutcomes(cfg, ChatGate(cfg, channels, notifykind.CLIMessage), statePath, text, telegramBaseURL)
}

// ChatGate is the configured/off/unchecked gate for Telegram and Slack, in
// that order. A channel it stops has its Outcome set; one it lets through has
// it empty. An empty kind skips the kind-list check, for system messages.
func ChatGate(cfg config.NotificationsConfig, channels config.NotificationChannelsConfig, kind string) []ChannelOutcome {
	out := []ChannelOutcome{{Channel: ChannelTelegram}, {Channel: ChannelSlack}}
	gates := []struct {
		configured bool
		ch         config.ChannelConfig
	}{
		{cfg.Telegram.BotToken != "", channels.Telegram},
		{cfg.Slack.WebhookURL != "", channels.Slack},
	}
	for i, g := range gates {
		switch {
		case !g.configured:
			out[i].Outcome = OutcomeUnconfigured
		case !g.ch.Enabled:
			out[i].Outcome = OutcomeOff
		case kind != "" && !slices.Contains(g.ch.Kinds, kind):
			out[i].Outcome = OutcomeUnchecked
		}
	}
	return out
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
// source "cli" — there's no ticket to pause a trip against, so pauseTicket is
// nil. A trip still writes its bookkeeping; the caller just skips sending.
func gateCLISend(statePath, transport string) (bool, error) {
	result, err := notificationGateAt(statePath, transport, notifyKindCLI, "cli", time.Now(), true, nil)
	if err != nil {
		return false, err
	}
	return result.Decision == Allowed, nil
}
