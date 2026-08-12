package ralphloop

import (
	"net/http"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
)

func TestSendMessage_NoneConfigured_NoOpsAndReturnsNoSent(t *testing.T) {
	t.Parallel()
	sent, err := sendMessage(config.NotificationsConfig{}, "hi", telegramAPIBaseURL)
	if err != nil {
		t.Fatalf("sendMessage: %v", err)
	}
	if len(sent) != 0 {
		t.Fatalf("sent = %v, want none", sent)
	}
}

func TestSendMessage_BothConfigured_SendsToBothAndReportsBoth(t *testing.T) {
	t.Parallel()
	telegramServer, getTelegramRequests := fakeTelegramServer(t, http.StatusOK)
	slackServer, getSlackRequests := fakeSlackServer(t, http.StatusOK)

	cfg := config.NotificationsConfig{
		Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "chat-1"},
		Slack:    config.SlackConfig{WebhookURL: slackServer.URL},
	}

	sent, err := sendMessage(cfg, "hello", telegramServer.URL)
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
	if want := telegramStyle.escape("hello"); telegramReqs[0].Text != want {
		t.Errorf("telegram text = %q, want %q", telegramReqs[0].Text, want)
	}

	slackReqs := getSlackRequests()
	if len(slackReqs) != 1 {
		t.Fatalf("slack requests = %v, want exactly 1", slackReqs)
	}
	if want := slackStyle.escape("hello"); slackReqs[0].Text != want {
		t.Errorf("slack text = %q, want %q", slackReqs[0].Text, want)
	}
}

func TestSendMessage_TelegramOnlyConfigured_SendsOnlyTelegram(t *testing.T) {
	t.Parallel()
	telegramServer, getTelegramRequests := fakeTelegramServer(t, http.StatusOK)
	cfg := config.NotificationsConfig{
		Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "chat-1"},
	}

	sent, err := sendMessage(cfg, "hello", telegramServer.URL)
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

	sent, err := sendMessage(cfg, "hi", telegramAPIBaseURL)
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

	sent, err := sendMessage(cfg, "hi", failingTelegram.URL)
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
