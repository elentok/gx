package config

// LogConfig holds log-view display settings.
type LogConfig struct {
	ImportantRefs []ImportantRefRule `json:"important-refs,omitempty"`
}

// ImportantRefRule matches refs by regex patterns and assigns them a highlight color.
type ImportantRefRule struct {
	Patterns []string `json:"patterns"`
	Color    string   `json:"color"`
}

// DefaultLogConfig returns the built-in important-refs preset.
func DefaultLogConfig() LogConfig {
	return LogConfig{
		ImportantRefs: []ImportantRefRule{
			{
				Patterns: []string{"^main$", "^master$", "^origin/main$", "^origin/master$"},
				Color:    "yellow",
			},
			{
				Patterns: []string{"^v\\d"},
				Color:    "blue",
			},
		},
	}
}
