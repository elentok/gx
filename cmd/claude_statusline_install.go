package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// claudeStatusLineCommand is the command installed into Claude Code's
// statusLine settings.
const claudeStatusLineCommand = "gx claude statusline"

// installClaudeStatusline idempotently sets ~/.claude/settings.json's
// "statusLine" entry to run claudeStatusLineCommand, preserving every other
// top-level key already in that file.
func installClaudeStatusline(d deps) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	path := filepath.Join(home, ".claude", "settings.json")

	settings, err := readClaudeSettings(path)
	if err != nil {
		return err
	}

	settings["statusLine"] = map[string]any{
		"type":    "command",
		"command": claudeStatusLineCommand,
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	fmt.Fprintf(d.stdout, "Installed statusLine into %s\n", path)
	return nil
}

// readClaudeSettings parses the settings file, treating a missing or empty
// file as an empty object.
func readClaudeSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return map[string]any{}, nil
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return settings, nil
}
