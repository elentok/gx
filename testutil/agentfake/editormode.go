package agentfake

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// editorModeOf reads editorMode from a settings file or inline JSON; "" when
// absent or unreadable.
func editorModeOf(settings string) string {
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
		EditorMode string `json:"editorMode"`
	}
	if json.Unmarshal([]byte(raw), &s) != nil {
		return ""
	}
	return s.EditorMode
}

// claudeSettingsPath is CLAUDE_CONFIG_DIR's settings.json, or ~/.claude's.
func claudeSettingsPath() string {
	cfgDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if cfgDir == "" {
		home, _ := os.UserHomeDir()
		cfgDir = filepath.Join(home, ".claude")
	}
	return filepath.Join(cfgDir, "settings.json")
}

// launchEditorMode is the editor mode the fake starts in. Like claude, a
// --settings flag beats the person's config; the last flag that sets one wins.
// The config is the global .claude.json, then settings.json. Like gx's
// claudeConfigDirs, CLAUDE_CONFIG_DIR holds both files when it is set; else
// they are HOME's.
func launchEditorMode(args []string) string {
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
