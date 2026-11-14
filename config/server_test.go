package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadServerLogRetention(t *testing.T) {
	tmp := t.TempDir()
	prev := userConfigDirFn
	userConfigDirFn = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userConfigDirFn = prev })

	dir := filepath.Join(tmp, "gx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"server":{"log-retention":"48h"}}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.LogRetention != 48*time.Hour {
		t.Fatalf("LogRetention = %v, want 48h", cfg.Server.LogRetention)
	}
}

func TestParseServerWatchConfig(t *testing.T) {
	for body, want := range map[string]bool{`{}`: true, `{"server":{}}`: true, `{"server":{"watch-config":false}}`: false, `{"server":{"watch-config":true}}`: true} {
		cfg, err := Parse([]byte(body))
		if err != nil || cfg.Server.WatchConfig != want {
			t.Errorf("%s: WatchConfig = %v (err %v), want %v", body, cfg.Server.WatchConfig, err, want)
		}
	}
}

func TestParseServerRecentlyFinished(t *testing.T) {
	for body, want := range map[string]int{`{}`: 5, `{"server":{"recently-finished":8}}`: 8, `{"server":{"recently-finished":-1}}`: 5} {
		cfg, err := Parse([]byte(body))
		if err != nil || cfg.Server.RecentlyFinished != want {
			t.Errorf("%s: RecentlyFinished = %d (err %v), want %d", body, cfg.Server.RecentlyFinished, err, want)
		}
	}
}

func TestLoadCPUWatchdog(t *testing.T) {
	def := DefaultCPUWatchdogConfig()
	for _, tc := range []struct {
		name, json string
		want       CPUWatchdogConfig
	}{
		{"absent", `{}`, def},
		{"set", `{"cpu-watchdog":{"enabled":false,"threshold-percent":40,"sustain":"5m","cooldown":"1h"}}`,
			CPUWatchdogConfig{ThresholdPercent: 40, Sustain: 5 * time.Minute, Cooldown: time.Hour}},
		{"invalid", `{"cpu-watchdog":{"threshold-percent":-1,"sustain":"soon","cooldown":"later"}}`, def},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			prev := userConfigDirFn
			userConfigDirFn = func() (string, error) { return tmp, nil }
			t.Cleanup(func() { userConfigDirFn = prev })
			dir := filepath.Join(tmp, "gx")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tc.json), 0644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.CPUWatchdog != tc.want {
				t.Fatalf("CPUWatchdog = %+v, want %+v", cfg.CPUWatchdog, tc.want)
			}
		})
	}
}

func TestLoadServerWebTOTP(t *testing.T) {
	for body, want := range map[string]bool{
		`{}`:                                true,
		`{"server":{"web":{}}}`:             true,
		`{"server":{"web":{"totp":true}}}`:  true,
		`{"server":{"web":{"totp":false}}}`: false,
	} {
		tmp := t.TempDir()
		prev := userConfigDirFn
		userConfigDirFn = func() (string, error) { return tmp, nil }
		t.Cleanup(func() { userConfigDirFn = prev })
		writeBudgetConfig(t, tmp, body)
		cfg, err := Load()
		if err != nil || cfg.Server.Web.TOTP != want {
			t.Errorf("%s: TOTP = %v (err %v), want %v", body, cfg.Server.Web.TOTP, err, want)
		}
	}
}

func TestLoadServerWebKeybindings(t *testing.T) {
	type keys struct{ mac, other, vi string }
	for body, want := range map[string]keys{
		`{}`:                                    {"cmd+e", "ctrl+e", ""},
		`{"server":{}}`:                         {"cmd+e", "ctrl+e", ""},
		`{"server":{"web":{"vi-escape":"jk"}}}`: {"cmd+e", "ctrl+e", "jk"},
		`{"server":{"web":{"leader-key-mac":"alt+j","leader-key":"ctrl+k"}}}`: {"alt+j", "ctrl+k", ""},
		`{"server":{"web":{"leader-key-mac":"","leader-key":""}}}`:            {"", "", ""},
	} {
		tmp := t.TempDir()
		prev := userConfigDirFn
		userConfigDirFn = func() (string, error) { return tmp, nil }
		t.Cleanup(func() { userConfigDirFn = prev })
		writeBudgetConfig(t, tmp, body)
		cfg, err := Load()
		got := keys{cfg.Server.Web.LeaderKeyMac, cfg.Server.Web.LeaderKey, cfg.Server.Web.ViEscape}
		if err != nil || got != want {
			t.Errorf("%s: got %+v (err %v), want %+v", body, got, err, want)
		}
	}
}

func TestLoadServerWebDev(t *testing.T) {
	type dev struct {
		on  bool
		dir string
	}
	for body, want := range map[string]dev{
		`{}`: {},
		`{"server":{"web":{"dev-frontend":true}}}`:                           {on: true},
		`{"server":{"web":{"dev-frontend":true,"dev-dir":"~/src/gx"}}}`:      {on: true, dir: "~/src/gx"},
		`{"server":{"web":{"dev-dir":"/opt/gx"}}}`:                           {dir: "/opt/gx"},
		`{"server":{"web":{"dev-frontend":true,"dev-url":"http://x:5174"}}}`: {on: true},
	} {
		tmp := t.TempDir()
		prev := userConfigDirFn
		userConfigDirFn = func() (string, error) { return tmp, nil }
		t.Cleanup(func() { userConfigDirFn = prev })
		writeBudgetConfig(t, tmp, body)
		cfg, err := Load()
		got := dev{cfg.Server.Web.DevFrontend, cfg.Server.Web.DevDir}
		if err != nil || got != want {
			t.Errorf("%s: dev = %+v (err %v), want %+v", body, got, err, want)
		}
	}
}
