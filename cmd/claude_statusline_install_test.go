package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallClaudeStatusline_MissingFileWritesCanonical(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	out := &strings.Builder{}
	d := deps{stdout: out, userHomeDir: func() (string, error) { return home, nil }}

	if err := installClaudeStatusline(d); err != nil {
		t.Fatalf("installClaudeStatusline: %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("written file is not valid JSON: %v\n%s", err, data)
	}
	statusLine, ok := got["statusLine"].(map[string]any)
	if !ok {
		t.Fatalf("written settings missing statusLine object: %#v", got)
	}
	if statusLine["command"] != claudeStatusLineCommand {
		t.Fatalf("statusLine command = %v, want %q", statusLine["command"], claudeStatusLineCommand)
	}
	if statusLine["type"] != "command" {
		t.Fatalf("statusLine type = %v, want %q", statusLine["type"], "command")
	}

	if !strings.Contains(out.String(), settingsPath) {
		t.Fatalf("expected confirmation to mention settings path, got %q", out.String())
	}
}

func TestInstallClaudeStatusline_PreservesOtherSettings(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	settingsDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	existing := `{"model":"sonnet","statusLine":{"type":"command","command":"old-command"}}`
	settingsPath := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write existing settings: %v", err)
	}

	d := deps{stdout: &strings.Builder{}, userHomeDir: func() (string, error) { return home, nil }}
	if err := installClaudeStatusline(d); err != nil {
		t.Fatalf("installClaudeStatusline: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("written file is not valid JSON: %v\n%s", err, data)
	}
	if got["model"] != "sonnet" {
		t.Fatalf("expected unrelated settings preserved, got %#v", got)
	}
	statusLine := got["statusLine"].(map[string]any)
	if statusLine["command"] != claudeStatusLineCommand {
		t.Fatalf("statusLine command = %v, want %q", statusLine["command"], claudeStatusLineCommand)
	}
}

func TestInstallClaudeStatusline_IdempotentAcrossRuns(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	d := deps{stdout: &strings.Builder{}, userHomeDir: func() (string, error) { return home, nil }}

	if err := installClaudeStatusline(d); err != nil {
		t.Fatalf("installClaudeStatusline (1st run): %v", err)
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	first, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after 1st run: %v", err)
	}

	if err := installClaudeStatusline(d); err != nil {
		t.Fatalf("installClaudeStatusline (2nd run): %v", err)
	}
	second, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after 2nd run: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("running --install twice was not idempotent:\n1st: %s\n2nd: %s", first, second)
	}
}
