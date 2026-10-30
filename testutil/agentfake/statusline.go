package agentfake

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"
)

// statusLineTimeout bounds the command so a hung one cannot stall the fake.
const statusLineTimeout = 10 * time.Second

// statusLineCommand returns the settings' statusLine command, or "" when the
// settings are absent or malformed.
func statusLineCommand(settings string) string {
	raw := strings.TrimSpace(settings)
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "{") {
		data, err := os.ReadFile(raw)
		if err != nil {
			return ""
		}
		raw = string(data)
	}
	var s struct {
		StatusLine struct {
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return ""
	}
	return s.StatusLine.Command
}

// StatusLinePayload is the JSON claude sends the statusLine command. Without
// usage there is no rate_limits key.
func StatusLinePayload(model string, usage *UsageConfig) ([]byte, error) {
	payload := map[string]any{
		"model": map[string]any{"display_name": model},
	}
	if usage != nil {
		payload["rate_limits"] = usage
	}
	return json.Marshal(payload)
}

// runStatusLine runs the settings' statusLine command after a turn. Like
// claude, it ignores the command's output and failure.
func (t *tui) runStatusLine() {
	command := statusLineCommand(t.opts.settings)
	if command == "" {
		return
	}
	var usage *UsageConfig
	if cfg, err := LoadWorkerConfig(t.pid.Cwd); err == nil {
		usage = cfg.Usage
	}
	payload, err := StatusLinePayload(t.opts.model, usage)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), statusLineTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = t.pid.Cwd
	cmd.Stdin = bytes.NewReader(payload)
	_ = cmd.Run()
}
