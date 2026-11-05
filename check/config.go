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
	"time"
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
	Limits  Limits
	// Always are extra commands that run in every non-full run, next to the
	// scoped command. A --full run already covers them.
	Always []string
}

// Limits are the optional time limits of a test entry. A zero duration means
// that limit is off.
type Limits struct {
	Suite time.Duration
	Hang  time.Duration
	Slow  SlowLimits
}

// SlowLimits is the per-test slow limit by brief tool name; a tool that is
// absent has no limit.
type SlowLimits map[string]time.Duration

// slowToolAliases maps the tool names a config spells to brief tool names.
var slowToolAliases = map[string]string{"go": "go-test", "vitest": "vitest", "playwright": "playwright"}

// kindTest is the check kind that runs tests, the only one with limits.
const kindTest = "test"

func parseLimits(raw json.RawMessage) (Limits, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return Limits{}, errors.New(`limits: want an object {"suite", "slow", "hang"}`)
	}
	var l Limits
	for _, key := range slices.Sorted(maps.Keys(m)) {
		var err error
		switch key {
		case "suite":
			l.Suite, err = parseDuration(key, m[key])
		case "hang":
			l.Hang, err = parseDuration(key, m[key])
		case "slow":
			l.Slow, err = parseSlow(m[key])
		default:
			err = fmt.Errorf("limits: unknown key %q (want suite, slow, hang)", key)
		}
		if err != nil {
			return Limits{}, err
		}
	}
	return l, nil
}

func parseSlow(raw json.RawMessage) (SlowLimits, error) {
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("limits.slow: %v", err)
		}
		s := SlowLimits{}
		for _, tool := range slices.Sorted(maps.Keys(m)) {
			d, err := parseDuration("slow."+tool, m[tool])
			if err != nil {
				return nil, err
			}
			name, ok := slowToolAliases[tool]
			if !ok {
				return nil, fmt.Errorf("limits.slow: unknown tool %q (want %s)", tool, strings.Join(slices.Sorted(maps.Keys(slowToolAliases)), ", "))
			}
			s[name] = d
		}
		return s, nil
	}
	d, err := parseDuration("slow", raw)
	if err != nil {
		return nil, err
	}
	s := SlowLimits{}
	for _, name := range slowToolAliases {
		s[name] = d
	}
	return s, nil
}

func parseDuration(key string, raw json.RawMessage) (time.Duration, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, fmt.Errorf("limits.%s: want a duration string like \"20s\"", key)
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("limits.%s: %q is not a positive duration like \"20s\"", key, s)
	}
	return d, nil
}

// RebuildHint is appended to an unknown-field or unknown-kind error: the usual
// cause is an installed gx older than the config's repo.
const RebuildHint = "this gx may be older than the repo; rebuild it with `go install github.com/elentok/gx@latest` (in the gx repo: `go install .`)"

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
			return nil, fmt.Errorf("unknown kind %q (want %s); %s", key, strings.Join(Kinds, ", "), RebuildHint)
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
			Changed *string         `json:"changed"`
			Full    *string         `json:"full"`
			Limits  json.RawMessage `json:"limits"`
			Always  json.RawMessage `json:"always"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&pair); err != nil {
			if strings.Contains(err.Error(), "unknown field") {
				return Command{}, fmt.Errorf("%v; %s", err, RebuildHint)
			}
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
		if pair.Limits != nil {
			l, err := parseLimits(pair.Limits)
			if err != nil {
				return Command{}, err
			}
			c.Limits = l
		}
		if pair.Always != nil {
			var always []string
			if err := json.Unmarshal(pair.Always, &always); err != nil {
				return Command{}, errors.New(`"always" must be a list of command strings`)
			}
			if slices.Contains(always, "") {
				return Command{}, errors.New(`"always" must not hold an empty command`)
			}
			c.Always = always
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
