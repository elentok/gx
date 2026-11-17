package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

var claudeEffortLevels = []string{"low", "medium", "high", "xhigh", "max"}

// ClaudeEffortLevel is the effort claude launches with when no effort is
// passed: the effortLevel its config holds (set by /effort), or "" when the
// config sets none or a value claude does not take. env is the launch env as
// LaunchEnv builds it; nil reads this process's.
func ClaudeEffortLevel(env []string) string {
	configDir, globalDir, err := claudeConfigDirs(env)
	if err != nil {
		return ""
	}
	level := ""
	// settings.json beats the global .claude.json.
	for _, path := range []string{filepath.Join(globalDir, ".claude.json"), filepath.Join(configDir, "settings.json")} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg struct {
			EffortLevel string `json:"effortLevel"`
		}
		if json.Unmarshal(data, &cfg) == nil && slices.Contains(claudeEffortLevels, cfg.EffortLevel) {
			level = cfg.EffortLevel
		}
	}
	return level
}
