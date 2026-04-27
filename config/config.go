package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var userConfigDirFn = os.UserConfigDir

const SchemaURL = "https://raw.githubusercontent.com/elentok/gx/main/docs/config-schema.json"

// Config is gx's user configuration.
type Config struct {
	Schema                string           `json:"$schema,omitempty"`
	UseNerdFontIcons      bool             `json:"use-nerdfont-icons"`
	StageDiffContextLines int              `json:"stage-diff-context-lines"`
	InputModalBottom      InputModalBottom `json:"input-modal-bottom"`
	NameAliases           map[string]string `json:"name-aliases,omitempty"`
}

// Default returns the default configuration.
func Default() Config {
	return Config{
		UseNerdFontIcons:      true,
		StageDiffContextLines: 1,
		InputModalBottom:      DefaultInputModalBottom(),
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
		StageDiffContextLines *int              `json:"stage-diff-context-lines"`
		InputModalBottom      *InputModalBottom `json:"input-modal-bottom"`
		NameAliases           map[string]string `json:"name-aliases"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if raw.UseNerdFontIcons != nil {
		cfg.UseNerdFontIcons = *raw.UseNerdFontIcons
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

	return cfg, nil
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
