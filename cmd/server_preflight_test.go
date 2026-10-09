package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
)

func TestPreflightAgentRunner(t *testing.T) {
	found := func(string) (string, error) { return "/bin/x", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }
	up := func(time.Duration) error { return nil }
	down := func(time.Duration) error { return errors.New("connection refused") }

	tests := []struct {
		name      string
		setting   string
		probe     agentrunner.Probe
		wantErr   string
		wantLevel string
	}{
		{"bad value", "tmux", agentrunner.Probe{LookPath: found, PingHerdr: up}, `invalid agent-runner "tmux"`, "ERROR"},
		{"explicit herdr down", "herdr", agentrunner.Probe{LookPath: found, PingHerdr: down}, "herdr is unavailable", "ERROR"},
		{"headless without claude", "headless", agentrunner.Probe{LookPath: missing, PingHerdr: up}, "needs claude on PATH", "ERROR"},
		{"auto picks herdr", "auto", agentrunner.Probe{LookPath: found, PingHerdr: up}, "", "INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "state", "server.log")
			_, err := preflightAgentRunner(tt.setting, tt.probe, logPath)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}

			data, rerr := os.ReadFile(logPath)
			if rerr != nil {
				t.Fatal(rerr)
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) != 1 {
				t.Fatalf("log lines = %d, want 1: %s", len(lines), data)
			}
			var rec map[string]any
			if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
				t.Fatalf("log line not JSON: %v", err)
			}
			if rec["level"] != tt.wantLevel || rec["setting"] != tt.setting {
				t.Errorf("log = %s, want level %s setting %s", lines[0], tt.wantLevel, tt.setting)
			}
			if tt.wantErr != "" && !strings.Contains(rec["err"].(string), tt.wantErr) {
				t.Errorf("log err = %v, want containing %q", rec["err"], tt.wantErr)
			}
		})
	}
}
