package ralphloop

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/config"
)

func withBudgetLogPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "budget-log.jsonl")
	previous := budgetLogPathFn
	budgetLogPathFn = func() (string, error) { return path, nil }
	t.Cleanup(func() { budgetLogPathFn = previous })
	return path
}

func TestSendBudgetNotification_EachKind_SendsAndLogsItsOwnMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	logPath := withBudgetLogPath(t)
	telegramServer, getTelegramRequests := fakeTelegramServer(t, http.StatusOK)
	cfg := config.NotificationsConfig{Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "chat-1"}}

	cases := []struct {
		kind BudgetEventKind
		text string
	}{
		{BudgetThresholdCrossed, "threshold crossed: $10.00"},
		{BudgetSoftLimitPaused, "soft limit paused"},
		{BudgetHardLimitKilled, "hard limit killed"},
	}

	for _, tc := range cases {
		sent, err := sendBudgetNotification(cfg, tc.kind, tc.text, telegramServer.URL)
		if err != nil {
			t.Fatalf("sendBudgetNotification(%s): %v", tc.kind, err)
		}
		if len(sent) != 1 || sent[0] != "telegram" {
			t.Fatalf("sent = %v, want [telegram]", sent)
		}
	}

	reqs := getTelegramRequests()
	if len(reqs) != len(cases) {
		t.Fatalf("telegram requests = %d, want %d", len(reqs), len(cases))
	}
	for i, tc := range cases {
		if want := telegramStyle.chatStyle.Escape(tc.text).String(); reqs[i].Text != want {
			t.Errorf("request %d text = %q, want %q", i, reqs[i].Text, want)
		}
	}

	events, err := ReadBudgetEvents(logPath)
	if err != nil {
		t.Fatalf("ReadBudgetEvents: %v", err)
	}
	if len(events) != len(cases) {
		t.Fatalf("events = %+v, want %d entries", events, len(cases))
	}
	for i, tc := range cases {
		if events[i].Kind != tc.kind || events[i].Body != tc.text {
			t.Errorf("event %d = %+v, want kind=%s body=%q", i, events[i], tc.kind, tc.text)
		}
		if len(events[i].Sent) != 1 || events[i].Sent[0] != "telegram" {
			t.Errorf("event %d Sent = %v, want [telegram]", i, events[i].Sent)
		}
	}
}

func TestSendBudgetNotification_NoTransportsConfigured_StillLogsEvent(t *testing.T) {
	logPath := withBudgetLogPath(t)

	sent, err := sendBudgetNotification(config.NotificationsConfig{}, BudgetThresholdCrossed, "hi", telegramAPIBaseURL)
	if err != nil {
		t.Fatalf("sendBudgetNotification: %v", err)
	}
	if len(sent) != 0 {
		t.Fatalf("sent = %v, want none", sent)
	}

	events, err := ReadBudgetEvents(logPath)
	if err != nil {
		t.Fatalf("ReadBudgetEvents: %v", err)
	}
	if len(events) != 1 || events[0].Body != "hi" {
		t.Fatalf("events = %+v, want one entry with body %q", events, "hi")
	}
}

// TestSendBudgetNotification_BypassesGlobalMuteGate pins ticket 05's
// deliberate gate bypass: an unrelated epic's own notification sink tripping
// the shared global mute/throttle breaker (see notification_gate.go) must
// never silently swallow a budget notification, since a budget event isn't
// attributable to any one epic.
func TestSendBudgetNotification_BypassesGlobalMuteGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	withBudgetLogPath(t)
	telegramServer, getTelegramRequests := fakeTelegramServer(t, http.StatusOK)
	cfg := config.NotificationsConfig{Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "chat-1"}}

	// Trip the global breaker via the gated CLI send path (an unrelated
	// source), same as TestSendMessage_GlobalBreakerTrips_SuppressesFurtherSends.
	for range globalThreshold + 5 {
		if _, err := sendMessage(cfg, "unrelated epic event", telegramServer.URL); err != nil {
			t.Fatalf("sendMessage: %v", err)
		}
	}
	tripped := len(getTelegramRequests())
	if tripped >= globalThreshold+5 {
		t.Fatalf("breaker did not trip: got %d requests", tripped)
	}

	sent, err := sendBudgetNotification(cfg, BudgetThresholdCrossed, "budget alert", telegramServer.URL)
	if err != nil {
		t.Fatalf("sendBudgetNotification: %v", err)
	}
	if len(sent) != 1 || sent[0] != "telegram" {
		t.Fatalf("sent = %v, want [telegram] despite tripped global gate", sent)
	}
	if got := len(getTelegramRequests()); got != tripped+1 {
		t.Fatalf("telegram requests after budget send = %d, want %d", got, tripped+1)
	}
}
