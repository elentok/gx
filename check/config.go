// Package check runs a project's check config (format, lint, typecheck, test)
// on the changed files or on the whole repo and reports a short result.
package check

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Kinds lists the check kinds in output order.
var Kinds = []string{"format", "lint", "typecheck", "test"}

// RepoConfigPath is the repo-relative config file.
const RepoConfigPath = ".gx/checks.json"

// Command is one kind's config. Changed is empty when the pair has no changed
// command, which means "use the gx default".
type Command struct {
	Off     bool
	Changed string
	Full    string
}

// Config maps a kind to its command; a missing kind is unconfigured.
type Config map[string]Command

// ParseConfig reads the checks JSON. The error is one line naming the problem.
func ParseConfig(data []byte) (Config, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("invalid JSON: %v", err)
	}
	cfg := Config{}
	for _, key := range slices.Sorted(maps.Keys(top)) {
		if key == "$schema" {
			continue
		}
		if !slices.Contains(Kinds, key) {
			return nil, fmt.Errorf("unknown kind %q (want %s)", key, strings.Join(Kinds, ", "))
		}
		c, err := parseCommand(top[key])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		cfg[key] = c
	}
	return cfg, nil
}

func parseCommand(raw json.RawMessage) (Command, error) {
	raw = bytes.TrimSpace(raw)
	switch {
	case string(raw) == "false":
		return Command{Off: true}, nil
	case bytes.HasPrefix(raw, []byte(`"`)):
		var s string
		if err := json.Unmarshal(raw, &s); err != nil || s == "" {
			return Command{}, errors.New("command must not be empty")
		}
		return Command{Changed: s, Full: s}, nil
	case bytes.HasPrefix(raw, []byte("{")):
		var pair struct {
			Changed *string `json:"changed"`
			Full    *string `json:"full"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&pair); err != nil {
			return Command{}, fmt.Errorf("want {\"changed\": string, \"full\": string}: %v", err)
		}
		if pair.Full == nil || *pair.Full == "" {
			return Command{}, errors.New(`"full" is required`)
		}
		c := Command{Full: *pair.Full}
		if pair.Changed != nil {
			if *pair.Changed == "" {
				return Command{}, errors.New(`"changed" must not be empty`)
			}
			c.Changed = *pair.Changed
		}
		return c, nil
	}
	return Command{}, errors.New(`want a command string, false or {"changed", "full"}`)
}

// LoadConfig resolves the config: the repo file, else projectChecks (called
// only when the file is missing), else none. The repo file wins.
func LoadConfig(root string, projectChecks func() (json.RawMessage, error)) (Config, error) {
	data, err := os.ReadFile(filepath.Join(root, RepoConfigPath))
	switch {
	case err == nil:
		cfg, err := ParseConfig(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", RepoConfigPath, err)
		}
		return cfg, nil
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	if projectChecks == nil {
		return Config{}, nil
	}
	raw, err := projectChecks()
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return Config{}, nil
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("project.json checks: %w", err)
	}
	return cfg, nil
}
