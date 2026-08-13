package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UserConfigDir hardcodes ~/.config as gx's config base directory on every
// platform, deliberately bypassing os.UserConfigDir's per-OS/XDG resolution
// (e.g. ~/Library/Application Support on macOS, or $XDG_CONFIG_HOME when
// set). It's the single source of truth other gx packages call into for
// this decision.
func UserConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

// UserCacheDir hardcodes ~/.cache as gx's cache base directory on every
// platform, mirroring UserConfigDir's deliberate bypass of per-OS/XDG
// resolution.
func UserCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache"), nil
}

// UserStateDir hardcodes ~/.local/state as gx's runtime-state base directory
// on every platform, mirroring UserConfigDir/UserCacheDir's deliberate
// bypass of per-OS/XDG resolution. Runtime state (queue-state.json,
// notifications-state.json) lives here rather than under UserConfigDir,
// which is reserved for user-edited config (config.json).
func UserStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state"), nil
}

var userConfigDirFn = UserConfigDir
var userStateDirFn = UserStateDir

const SchemaURL = "https://raw.githubusercontent.com/elentok/gx/main/docs/config-schema.json"

// Config is gx's user configuration.
type Config struct {
	Schema                string               `json:"$schema,omitempty"`
	UseNerdFontIcons      bool                 `json:"use-nerdfont-icons"`
	ImageDiffs            bool                 `json:"image-diffs"`
	StageDiffContextLines int                  `json:"stage-diff-context-lines"`
	InputModalBottom      InputModalBottom     `json:"input-modal-bottom"`
	NameAliases           map[string]string    `json:"name-aliases,omitempty"`
	Log                   LogConfig            `json:"log,omitempty"`
	ExecutionQueue        ExecutionQueueConfig `json:"execution-queue"`
	Notifications         NotificationsConfig  `json:"notifications"`
	Skills                SkillsConfig         `json:"skills"`
	Agents                AgentsConfig         `json:"agents"`
}

// Default returns the default configuration.
func Default() Config {
	return Config{
		UseNerdFontIcons:      true,
		ImageDiffs:            true,
		StageDiffContextLines: 1,
		InputModalBottom:      DefaultInputModalBottom(),
		Log:                   DefaultLogConfig(),
		ExecutionQueue:        DefaultExecutionQueueConfig(),
		Notifications:         DefaultNotificationsConfig(),
		Skills:                DefaultSkillsConfig(),
		Agents:                DefaultAgentsConfig(),
	}
}

// FilePath returns the config file path, typically ~/.config/gx/config.json.
func FilePath() (string, error) {
	base, err := userConfigDirFn()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "gx", "config.json"), nil
}

// Load reads user config from disk. Missing file returns defaults.
func Load() (Config, error) {
	cfg := Default()
	path, err := FilePath()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}

	var raw struct {
		UseNerdFontIcons      *bool             `json:"use-nerdfont-icons"`
		ImageDiffs            *bool             `json:"image-diffs"`
		StageDiffContextLines *int              `json:"stage-diff-context-lines"`
		InputModalBottom      *InputModalBottom `json:"input-modal-bottom"`
		NameAliases           map[string]string `json:"name-aliases"`
		Log                   *LogConfig        `json:"log"`
		ExecutionQueue        *struct {
			MaxConcurrentTicketsPerEpic *int `json:"max-concurrent-tickets-per-epic"`
			MaxConcurrentEpics          *int `json:"max-concurrent-epics"`
		} `json:"execution-queue"`
		Notifications *struct {
			Telegram *struct {
				BotToken *string `json:"bot-token"`
				ChatID   *string `json:"chat-id"`
			} `json:"telegram"`
			Slack *struct {
				WebhookURL *string `json:"webhook-url"`
			} `json:"slack"`
		} `json:"notifications"`
		Skills *struct {
			Implement  *string  `json:"implement"`
			CodeReview []string `json:"code-review"`
		} `json:"skills"`
		Agents *struct {
			Claude *struct {
				Model  *string `json:"model"`
				Effort *string `json:"effort"`
			} `json:"claude"`
			Codex *struct {
				Model  *string `json:"model"`
				Effort *string `json:"effort"`
			} `json:"codex"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if raw.UseNerdFontIcons != nil {
		cfg.UseNerdFontIcons = *raw.UseNerdFontIcons
	}
	if raw.ImageDiffs != nil {
		cfg.ImageDiffs = *raw.ImageDiffs
	}
	if raw.StageDiffContextLines != nil {
		cfg.StageDiffContextLines = clampStageDiffContext(*raw.StageDiffContextLines)
	}
	if raw.InputModalBottom != nil {
		cfg.InputModalBottom = *raw.InputModalBottom
	}
	if raw.NameAliases != nil {
		cfg.NameAliases = make(map[string]string, len(raw.NameAliases))
		for k, v := range raw.NameAliases {
			cfg.NameAliases[k] = v
		}
	}
	if raw.Log != nil {
		if raw.Log.ImportantRefs != nil {
			cfg.Log.ImportantRefs = raw.Log.ImportantRefs
		}
		if raw.Log.HideRefs != nil {
			cfg.Log.HideRefs = raw.Log.HideRefs
		}
	}
	if raw.ExecutionQueue != nil {
		if raw.ExecutionQueue.MaxConcurrentTicketsPerEpic != nil {
			cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic = clampExecutionQueueLimit(*raw.ExecutionQueue.MaxConcurrentTicketsPerEpic)
		}
		if raw.ExecutionQueue.MaxConcurrentEpics != nil {
			cfg.ExecutionQueue.MaxConcurrentEpics = clampExecutionQueueLimit(*raw.ExecutionQueue.MaxConcurrentEpics)
		}
	}
	if raw.Notifications != nil && raw.Notifications.Telegram != nil {
		if raw.Notifications.Telegram.BotToken != nil {
			cfg.Notifications.Telegram.BotToken = *raw.Notifications.Telegram.BotToken
		}
		if raw.Notifications.Telegram.ChatID != nil {
			cfg.Notifications.Telegram.ChatID = *raw.Notifications.Telegram.ChatID
		}
	}
	if raw.Notifications != nil && raw.Notifications.Slack != nil {
		if raw.Notifications.Slack.WebhookURL != nil {
			cfg.Notifications.Slack.WebhookURL = *raw.Notifications.Slack.WebhookURL
		}
	}
	if raw.Skills != nil {
		if raw.Skills.Implement != nil {
			cfg.Skills.Implement = *raw.Skills.Implement
		}
		if raw.Skills.CodeReview != nil {
			cfg.Skills.CodeReview = raw.Skills.CodeReview
		}
	}
	if raw.Agents != nil {
		if raw.Agents.Claude != nil {
			applyAgentConfig(&cfg.Agents.Claude, raw.Agents.Claude.Model, raw.Agents.Claude.Effort)
		}
		if raw.Agents.Codex != nil {
			applyAgentConfig(&cfg.Agents.Codex, raw.Agents.Codex.Model, raw.Agents.Codex.Effort)
		}
	}

	return cfg, nil
}

// applyAgentConfig overlays explicitly-set model/effort values onto an
// AgentConfig, trimming whitespace and leaving unset keys at the built-in
// default. A trimmed-empty value survives as empty (meaning "inherit"),
// distinct from an absent key (meaning "use the default").
func applyAgentConfig(cfg *AgentConfig, model, effort *string) {
	if model != nil {
		cfg.Model = strings.TrimSpace(*model)
	}
	if effort != nil {
		cfg.Effort = strings.TrimSpace(*effort)
	}
}

func clampStageDiffContext(n int) int {
	if n < 0 {
		return 0
	}
	if n > 20 {
		return 20
	}
	return n
}

// Init writes the default config file and returns its path.
// It returns an error if the file already exists.
func Init() (string, error) {
	path, err := FilePath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("config already exists at %s", path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat config %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}

	cfg := Default()
	cfg.Schema = SchemaURL
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode default config: %w", err)
	}
	b = append(b, '\n')

	if err := os.WriteFile(path, b, 0644); err != nil {
		return "", fmt.Errorf("write config %s: %w", path, err)
	}
	return path, nil
}
