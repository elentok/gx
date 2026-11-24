package notify

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/notifykind"
)

func TestSendMessage_NoneConfigured_NoOpsAndReturnsNoSent(t *testing.T) {
	t.Parallel()
	sent, err := sendMessage(config.NotificationsConfig{}, testStatePath(t), "hi", telegramAPIBaseURL)
	if err != nil {
		t.Fatalf("sendMessage: %v", err)
	}
	if len(sent) != 0 {
		t.Fatalf("sent = %v, want none", sent)
	}
}

func TestSendMessage_CLIChannelsApplySwitchKindListAndMute(t *testing.T) {
	t.Parallel()
	on := config.ChannelConfig{Enabled: true, Kinds: []string{notifykind.CLIMessage}}
	for _, tc := range []struct {
		name  string
		slack config.ChannelConfig
		mute  bool
		want  string
		posts int
	}{
		{"on", on, false, OutcomeSent, 1},
		{"switch off", config.ChannelConfig{Kinds: on.Kinds}, false, OutcomeOff, 0},
		{"kind unchecked", config.ChannelConfig{Enabled: true, Kinds: []string{}}, false, OutcomeUnchecked, 0},
		{"manual mute", on, true, OutcomeMuted, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			slackServer, getSlackRequests := fakeSlackServer(t, http.StatusOK)
			statePath := testStatePath(t)
			if tc.mute {
				if err := UpdateNotificationState(statePath, func(s *NotificationState) {
					s.Transports["slack"] = TransportState{Muted: true, Reason: "manual-disable"}
				}); err != nil {
					t.Fatal(err)
				}
			}
			cfg := config.NotificationsConfig{Slack: config.SlackConfig{WebhookURL: slackServer.URL}}

			got, err := sendCLIMessage(cfg, config.NotificationChannelsConfig{Telegram: on, Slack: tc.slack}, statePath, "hi", telegramAPIBaseURL)

			if err != nil {
				t.Fatalf("sendCLIMessage: %v", err)
			}
			want := []ChannelOutcome{{ChannelTelegram, OutcomeUnconfigured}, {ChannelSlack, tc.want}}
			if !slices.Equal(got, want) {
				t.Errorf("outcomes = %v, want %v", got, want)
			}
			if n := len(getSlackRequests()); n != tc.posts {
				t.Errorf("slack requests = %d, want %d", n, tc.posts)
			}
		})
	}
}

func TestSendCLIMessage_TelegramErrorMentioningSlackIsBlamedOnTelegramOnly(t *testing.T) {
	t.Parallel()
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "slack: boom", http.StatusInternalServerError)
	}))
	t.Cleanup(failing.Close)
	slackServer, _ := fakeSlackServer(t, http.StatusOK)
	statePath := testStatePath(t)
	if err := UpdateNotificationState(statePath, func(s *NotificationState) {
		s.Transports["slack"] = TransportState{Muted: true, Reason: "manual-disable"}
	}); err != nil {
		t.Fatal(err)
	}
	on := config.ChannelConfig{Enabled: true, Kinds: []string{notifykind.CLIMessage}}
	cfg := config.NotificationsConfig{
		Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "chat-1"},
		Slack:    config.SlackConfig{WebhookURL: slackServer.URL},
	}

	got, err := sendCLIMessage(cfg, config.NotificationChannelsConfig{Telegram: on, Slack: on}, statePath, "hi", failing.URL)

	if err == nil {
		t.Fatal("sendCLIMessage: want error, got nil")
	}
	want := []ChannelOutcome{{ChannelTelegram, OutcomeFailed}, {ChannelSlack, OutcomeMuted}}
	if !slices.Equal(got, want) {
		t.Errorf("outcomes = %v, want %v", got, want)
	}
}

func testStatePath(t *testing.T) string {
	t.Helper()
	return NotificationStatePath(t.TempDir())
}

func TestSendMessage_BothConfigured_SendsToBothAndReportsBoth(t *testing.T) {
	t.Parallel()
	telegramServer, getTelegramRequests := fakeTelegramServer(t, http.StatusOK)
	slackServer, getSlackRequests := fakeSlackServer(t, http.StatusOK)

	cfg := config.NotificationsConfig{
		Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "chat-1"},
		Slack:    config.SlackConfig{WebhookURL: slackServer.URL},
	}

	sent, err := sendMessage(cfg, testStatePath(t), "hello", telegramServer.URL)
	if err != nil {
		t.Fatalf("sendMessage: %v", err)
	}
	if len(sent) != 2 || sent[0] != "telegram" || sent[1] != "slack" {
		t.Fatalf("sent = %v, want [telegram slack]", sent)
	}

	telegramReqs := getTelegramRequests()
	if len(telegramReqs) != 1 {
		t.Fatalf("telegram requests = %v, want exactly 1", telegramReqs)
	}
	if want := telegramStyle.chatStyle.Escape("hello").String(); telegramReqs[0].Text != want {
		t.Errorf("telegram text = %q, want %q", telegramReqs[0].Text, want)
	}

	slackReqs := getSlackRequests()
	if len(slackReqs) != 1 {
		t.Fatalf("slack requests = %v, want exactly 1", slackReqs)
	}
	if want := slackStyle.chatStyle.Escape("hello").String(); slackReqs[0].Text != want {
		t.Errorf("slack text = %q, want %q", slackReqs[0].Text, want)
	}
}

func TestSendMessage_TelegramOnlyConfigured_SendsOnlyTelegram(t *testing.T) {
	t.Parallel()
	telegramServer, getTelegramRequests := fakeTelegramServer(t, http.StatusOK)
	cfg := config.NotificationsConfig{
		Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "chat-1"},
	}

	sent, err := sendMessage(cfg, testStatePath(t), "hello", telegramServer.URL)
	if err != nil {
		t.Fatalf("sendMessage: %v", err)
	}
	if len(sent) != 1 || sent[0] != "telegram" {
		t.Fatalf("sent = %v, want [telegram]", sent)
	}
	if reqs := getTelegramRequests(); len(reqs) != 1 {
		t.Fatalf("telegram requests = %v, want exactly 1", reqs)
	}
}

func TestSendMessage_SlackFailure_ReturnsErrorMentioningSlack(t *testing.T) {
	t.Parallel()
	server, _ := fakeSlackServer(t, http.StatusInternalServerError)
	cfg := config.NotificationsConfig{Slack: config.SlackConfig{WebhookURL: server.URL}}

	sent, err := sendMessage(cfg, testStatePath(t), "hi", telegramAPIBaseURL)
	if err == nil {
		t.Fatal("sendMessage: want error, got nil")
	}
	if !strings.Contains(err.Error(), "slack") {
		t.Errorf("error = %q, want it to mention slack", err.Error())
	}
	if len(sent) != 0 {
		t.Fatalf("sent = %v, want none", sent)
	}
}

func TestSendMessage_TelegramFailsSlackSucceeds_ReportsSlackAndErrorsOnTelegram(t *testing.T) {
	t.Parallel()
	failingTelegram, _ := fakeTelegramServer(t, http.StatusInternalServerError)
	slackServer, _ := fakeSlackServer(t, http.StatusOK)
	cfg := config.NotificationsConfig{
		Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "chat-1"},
		Slack:    config.SlackConfig{WebhookURL: slackServer.URL},
	}

	sent, err := sendMessage(cfg, testStatePath(t), "hi", failingTelegram.URL)
	if err == nil {
		t.Fatal("sendMessage: want error, got nil")
	}
	if !strings.Contains(err.Error(), "telegram") {
		t.Errorf("error = %q, want it to mention telegram", err.Error())
	}
	if len(sent) != 1 || sent[0] != "slack" {
		t.Fatalf("sent = %v, want [slack] despite telegram failure", sent)
	}
}

func TestSendMessage_GlobalBreakerTrips_SuppressesFurtherSends(t *testing.T) {
	t.Parallel()
	statePath := testStatePath(t)
	telegramServer, getTelegramRequests := fakeTelegramServer(t, http.StatusOK)
	cfg := config.NotificationsConfig{
		Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "chat-1"},
	}

	for range globalThreshold + 5 {
		if _, err := sendMessage(cfg, statePath, "hi", telegramServer.URL); err != nil {
			t.Fatalf("sendMessage: %v", err)
		}
	}

	want := globalThreshold - 1
	if reqs := getTelegramRequests(); len(reqs) != want {
		t.Fatalf("telegram requests = %d, want exactly %d (breaker trips before the send that crosses the threshold)", len(reqs), want)
	}
}
