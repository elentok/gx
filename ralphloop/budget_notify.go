package ralphloop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/config"
)

// BudgetEventKind tags which of the three budget notification kinds a
// SendBudgetNotification call is producing. Each kind gets its own text
// template (the caller's responsibility — e.g. threshold-crossed's is
// ui/tickets' budgetThresholdCrossedText) but shares this one send/log
// mechanism.
type BudgetEventKind string

const (
	BudgetThresholdCrossed BudgetEventKind = "threshold-crossed"
	BudgetSoftLimitPaused  BudgetEventKind = "soft-limit-paused"
	BudgetHardLimitKilled  BudgetEventKind = "hard-limit-killed"
)

// SendBudgetNotification sends text to every notification service
// configured in cfg and durably logs the event, deliberately bypassing
// NotificationGate: a budget event is a sum across every running epic, not
// attributable to any one of them, so it must never be silently swallowed
// by an unrelated epic having already tripped that gate. The log write
// happens even if every transport is unconfigured or every send fails, so a
// budget action that fires while nobody's watching chat or the TUI still
// leaves a trace.
func SendBudgetNotification(cfg config.NotificationsConfig, kind BudgetEventKind, text string) ([]string, error) {
	return sendBudgetNotification(cfg, kind, text, telegramAPIBaseURL)
}

func sendBudgetNotification(cfg config.NotificationsConfig, kind BudgetEventKind, text, telegramBaseURL string) ([]string, error) {
	sent, sendErr := sendBudgetTransports(cfg, text, telegramBaseURL)
	logErr := logBudgetEvent(kind, text, sent)
	switch {
	case sendErr != nil && logErr != nil:
		return sent, fmt.Errorf("%w; %w", sendErr, logErr)
	case sendErr != nil:
		return sent, sendErr
	default:
		return sent, logErr
	}
}

// sendBudgetTransports sends text to every configured transport directly
// (no NotificationGate call), same per-service no-op-when-unconfigured
// shape as sendMessage.
func sendBudgetTransports(cfg config.NotificationsConfig, text, telegramBaseURL string) ([]string, error) {
	var sent []string
	var errs []error

	if cfg.Telegram.BotToken != "" {
		if err := sendTelegramMessage(cfg.Telegram.BotToken, cfg.Telegram.ChatID, telegramBaseURL, text); err != nil {
			errs = append(errs, fmt.Errorf("telegram: %w", err))
		} else {
			sent = append(sent, "telegram")
		}
	}

	if cfg.Slack.WebhookURL != "" {
		if err := SendSlackMessage(cfg.Slack.WebhookURL, text); err != nil {
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

// BudgetEvent is one line of budget-log.jsonl: a single threshold-crossed/
// soft-paused/hard-killed occurrence, session-level rather than attributed
// to any one epic (parallel to an epic's run-log.jsonl, see eventlog.go).
type BudgetEvent struct {
	Time time.Time       `json:"time"`
	Kind BudgetEventKind `json:"kind"`
	Body string          `json:"body"`
	// Sent names which transports the notification actually went out on
	// ("telegram", "slack") — possibly empty when no transport is
	// configured or every send failed, in which case this line is the
	// event's only surviving trace.
	Sent []string `json:"sent,omitempty"`
}

// budgetLogPathFn resolves budget-log.jsonl's real on-disk path, overridden
// in tests so they never touch the real machine's state file.
var budgetLogPathFn = budgetLogPath

// budgetLogPath returns budget-log.jsonl's path, under config.UserStateDir's
// ~/.local/state/gx/ layout (mirroring notifications-state.json).
func budgetLogPath() (string, error) {
	base, err := config.UserStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gx", "budget-log.jsonl"), nil
}

// budgetLogMu serializes appends across every goroutine in the process, so
// two events never interleave their JSON onto the same line.
var budgetLogMu sync.Mutex

func logBudgetEvent(kind BudgetEventKind, text string, sent []string) error {
	path, err := budgetLogPathFn()
	if err != nil {
		return err
	}
	return logBudgetEventAt(path, kind, text, sent, time.Now())
}

func logBudgetEventAt(path string, kind BudgetEventKind, text string, sent []string, now time.Time) error {
	data, err := json.Marshal(BudgetEvent{Time: now, Kind: kind, Body: text, Sent: sent})
	if err != nil {
		return err
	}
	data = append(data, '\n')

	budgetLogMu.Lock()
	defer budgetLogMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// ReadBudgetEvents reads and parses every line of budget-log.jsonl at path,
// skipping malformed lines rather than failing the whole read (mirroring
// ReadEvents). Returns no error and a nil slice if the file doesn't exist
// yet.
func ReadBudgetEvents(path string) ([]BudgetEvent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var events []BudgetEvent
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev BudgetEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, nil
}
