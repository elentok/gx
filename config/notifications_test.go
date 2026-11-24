package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadTelegramNotifications(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"notifications":{"telegram":{"bot-token":"abc123","chat-id":"456"}}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Notifications.Telegram.BotToken != "abc123" {
		t.Fatalf("BotToken = %q, want %q", cfg.Notifications.Telegram.BotToken, "abc123")
	}
	if cfg.Notifications.Telegram.ChatID != "456" {
		t.Fatalf("ChatID = %q, want %q", cfg.Notifications.Telegram.ChatID, "456")
	}
}

func TestLoadTelegramNotificationsMissingUsesDefaults(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Notifications.Telegram.BotToken != "" {
		t.Fatalf("BotToken = %q, want empty", cfg.Notifications.Telegram.BotToken)
	}
	if cfg.Notifications.Telegram.ChatID != "" {
		t.Fatalf("ChatID = %q, want empty", cfg.Notifications.Telegram.ChatID)
	}
}

func TestNotificationChannels_NewInstallDefaults(t *testing.T) {
	cfg, err := Parse([]byte(`{}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	nc := cfg.NotificationChannels
	for name, ch := range map[string]ChannelConfig{"desktop": nc.Desktop, "web-toast": nc.WebToast, "telegram": nc.Telegram, "slack": nc.Slack, "push": nc.Push} {
		if !ch.Enabled {
			t.Errorf("%s is off by default", name)
		}
		if len(ch.Kinds) != 15 || slices.Contains(ch.Kinds, "long-ticket") {
			t.Errorf("%s kinds = %v, want 15 ids without long-ticket", name, ch.Kinds)
		}
	}
}

func TestNotificationChannels_ExplicitValuesKept(t *testing.T) {
	cfg, err := Parse([]byte(`{"notification-channels":{
		"push":{"enabled":false,"kinds":["long-ticket","disk"]},
		"slack":{"kinds":[]}}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	nc := cfg.NotificationChannels
	if nc.Push.Enabled || !slices.Equal(nc.Push.Kinds, []string{"long-ticket", "disk"}) {
		t.Errorf("push = %+v", nc.Push)
	}
	if nc.Slack.Kinds == nil || len(nc.Slack.Kinds) != 0 || !nc.Slack.Enabled {
		t.Errorf("slack = %+v, want enabled with a non-nil empty list", nc.Slack)
	}
	if len(nc.Desktop.Kinds) != 15 {
		t.Errorf("an absent channel lost its default: %v", nc.Desktop.Kinds)
	}
}

func TestNotificationChannels_UnknownIDIsRefused(t *testing.T) {
	_, err := Parse([]byte(`{"notification-channels":{"web-toast":{"kinds":["disk","bogus"]}}}`))
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"notification-channels.web-toast.kinds", `"bogus"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func TestLoadSlackNotifications(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"notifications":{"slack":{"webhook-url":"https://hooks.example.com/x"}}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Notifications.Slack.WebhookURL != "https://hooks.example.com/x" {
		t.Fatalf("WebhookURL = %q, want %q", cfg.Notifications.Slack.WebhookURL, "https://hooks.example.com/x")
	}
}

func TestLoadSlackNotificationsMissingUsesDefaults(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Notifications.Slack.WebhookURL != "" {
		t.Fatalf("WebhookURL = %q, want empty", cfg.Notifications.Slack.WebhookURL)
	}
}
