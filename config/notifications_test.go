package config

import (
	"os"
	"path/filepath"
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
