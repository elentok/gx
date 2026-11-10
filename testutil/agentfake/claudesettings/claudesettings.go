// Package claudesettings reads the parts of claude's settings the fake honours.
package claudesettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// decode reads settings, a file path or inline JSON, into v. It reports false
// when the settings are absent or unreadable.
func decode(settings string, v any) bool {
	raw := strings.TrimSpace(settings)
	if raw == "" {
		return false
	}
	if !strings.HasPrefix(raw, "{") {
		data, err := os.ReadFile(raw)
		if err != nil {
			return false
		}
		raw = string(data)
	}
	return json.Unmarshal([]byte(raw), v) == nil
}

// editorModeOf reads editorMode from a settings file or inline JSON; "" when
// absent or unreadable.
func editorModeOf(settings string) string {
	var s struct {
		EditorMode string `json:"editorMode"`
	}
	if !decode(settings, &s) {
		return ""
	}
	return s.EditorMode
}

// StatusLineCommand returns the settings' statusLine command, or "" when the
// settings are absent or malformed.
func StatusLineCommand(settings string) string {
	var s struct {
		StatusLine struct {
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if !decode(settings, &s) {
		return ""
	}
	return s.StatusLine.Command
}

// Path is CLAUDE_CONFIG_DIR's settings.json, or ~/.claude's.
func Path() string {
	cfgDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if cfgDir == "" {
		home, _ := os.UserHomeDir()
		cfgDir = filepath.Join(home, ".claude")
	}
	return filepath.Join(cfgDir, "settings.json")
}

// LaunchEditorMode is the editor mode the fake starts in. Like claude, a
// --settings flag beats the person's config; the last flag that sets one wins.
// The config is the global .claude.json, then settings.json. Like gx's
// claudeConfigDirs, CLAUDE_CONFIG_DIR holds both files when it is set; else
// they are HOME's.
func LaunchEditorMode(args []string) string {
	mode := ""
	home, _ := os.UserHomeDir()
	cfgDir, globalDir := os.Getenv("CLAUDE_CONFIG_DIR"), home
	if cfgDir == "" {
		cfgDir = filepath.Join(home, ".claude")
	} else {
		globalDir = cfgDir
	}
	for _, path := range []string{filepath.Join(globalDir, ".claude.json"), filepath.Join(cfgDir, "settings.json")} {
		if m := editorModeOf(path); m != "" {
			mode = m
		}
	}
	for i, a := range args {
		v, ok := "", false
		switch {
		case a == "--settings" && i+1 < len(args):
			v, ok = args[i+1], true
		case strings.HasPrefix(a, "--settings="):
			v, ok = strings.TrimPrefix(a, "--settings="), true
		}
		if m := editorModeOf(v); ok && m != "" {
			mode = m
		}
	}
	return mode
}
