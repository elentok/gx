package agentfake

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CommandsFileEnv names a JSON file (see ProbeConfig) holding what the list
// probes answer. <cwd>/CommandsCwdFile is merged on top, so one worktree can
// differ from the rest.
const CommandsFileEnv = "GX_AGENTFAKE_COMMANDS_FILE"

// CommandsCwdFile is the per-directory override of the global commands file.
const CommandsCwdFile = ".agentfake-commands.json"

// CodexEnv, set to "1", makes a test binary that calls TUICodex from its
// TestMain act as interactive Codex on a PTY.
const CodexEnv = "GX_AGENTFAKE_CODEX"

// ProbeEntry is one command (fakeclaude) or skill (fakecodex) to list.
type ProbeEntry struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	ArgumentHint string   `json:"argumentHint,omitempty"`
	Aliases      []string `json:"aliases,omitempty"`
	Builtin      bool     `json:"builtin,omitempty"`
	// Disabled makes a fakecodex skill enabled:false.
	Disabled bool `json:"disabled,omitempty"`
}

// ProbeConfig is the commands file. A later file replaces Entries when it has
// any, Delay when set, and Fail when true.
type ProbeConfig struct {
	Entries []ProbeEntry `json:"entries,omitempty"`
	// Delay is a Go duration ("300ms") waited before the reply.
	Delay string `json:"delay,omitempty"`
	// Fail makes the probe exit 1 without a reply.
	Fail bool `json:"fail,omitempty"`
	// Models is the model list both fakes answer the model query with; a later
	// file replaces it when it has any. A fake with none sends no list.
	Models []ProbeModel `json:"models,omitempty"`
	// ModelsFail makes the model query fail while the commands probe still
	// answers.
	ModelsFail bool `json:"modelsFail,omitempty"`
}

func loadProbeConfig() (ProbeConfig, error) {
	var cfg ProbeConfig
	paths := []string{os.Getenv(CommandsFileEnv)}
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, CommandsCwdFile))
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return cfg, err
		}
		var layer ProbeConfig
		if err := json.Unmarshal(data, &layer); err != nil {
			return cfg, fmt.Errorf("agentfake: %s: %w", p, err)
		}
		if len(layer.Entries) > 0 {
			cfg.Entries = layer.Entries
		}
		if layer.Delay != "" {
			cfg.Delay = layer.Delay
		}
		cfg.Fail = cfg.Fail || layer.Fail
		if len(layer.Models) > 0 {
			cfg.Models = layer.Models
		}
		cfg.ModelsFail = cfg.ModelsFail || layer.ModelsFail
	}
	return cfg, nil
}

// wait sleeps the configured delay, then reports whether the probe must fail.
func (c ProbeConfig) wait() (fail bool, err error) {
	if c.Delay != "" {
		d, err := time.ParseDuration(c.Delay)
		if err != nil {
			return false, err
		}
		time.Sleep(d)
	}
	return c.Fail, nil
}

// isClaudeProbe is true for the non-interactive launch commandlist.ProbeClaude
// makes.
func IsClaudeProbe(args []string) bool {
	return slices.Contains(args, "-p")
}

// runClaudeProbe answers the stream-json initialize request with the
// configured commands and exits. It writes no transcript or session record.
func RunClaudeProbe() (int, error) {
	cfg, err := loadProbeConfig()
	if err != nil {
		return 0, err
	}
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return 0, errors.New("fakeclaude probe: no initialize request")
	}
	var req struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
		return 0, fmt.Errorf("fakeclaude probe: bad request: %w", err)
	}
	fail, err := cfg.wait()
	if err != nil {
		return 0, err
	}
	if fail {
		return 0, errors.New("fakeclaude probe: configured to fail")
	}
	commands := cfg.Entries
	if commands == nil {
		commands = []ProbeEntry{}
	}
	reply := map[string]any{"commands": commands, "agents": []any{}}
	// One initialize reply carries both lists, so each launch is one query.
	if cfg.Models != nil || cfg.ModelsFail {
		if err := LogModelQuery("claude"); err != nil {
			return 0, err
		}
	}
	if cfg.Models != nil && !cfg.ModelsFail {
		models := make([]map[string]any, 0, len(cfg.Models))
		for _, m := range cfg.Models {
			row := map[string]any{"value": m.Value, "resolvedModel": m.ID}
			if m.Efforts != nil {
				row["supportedEffortLevels"] = m.Efforts
			}
			models = append(models, row)
		}
		reply["models"] = models
	}
	out := json.NewEncoder(os.Stdout)
	for _, line := range []any{
		map[string]any{"type": "system", "subtype": "hook_started", "hook_name": "SessionStart:startup"},
		map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "success", "request_id": req.RequestID,
			"response": reply,
		}},
	} {
		if err := out.Encode(line); err != nil {
			return 0, err
		}
	}
	return 0, nil
}

// RunCodexProbe speaks the slice of `codex app-server` that
// commandlist.ProbeCodexSkills uses: initialize, initialized, skills/list.
func RunCodexProbe() (int, error) {
	cfg, err := loadProbeConfig()
	if err != nil {
		return 0, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	out := json.NewEncoder(os.Stdout)
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var req struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil || req.ID == nil {
			continue
		}
		switch req.Method {
		case "initialize":
			err = out.Encode(map[string]any{"id": *req.ID, "result": map[string]any{
				"userAgent": "agentfake/0", "codexHome": "", "platformFamily": "unix", "platformOs": "macos"}})
		case "model/list":
			if err = LogModelQuery("codex"); err != nil {
				return 0, err
			}
			if cfg.ModelsFail {
				err = out.Encode(map[string]any{"id": *req.ID, "error": map[string]any{"code": -32000, "message": "fakecodex: model/list configured to fail"}})
				break
			}
			var params struct {
				Cursor string `json:"cursor"`
			}
			_ = json.Unmarshal(req.Params, &params)
			page, _ := strconv.Atoi(strings.TrimPrefix(params.Cursor, "page-"))
			rows, last := []map[string]any{}, 0
			for _, m := range cfg.Models {
				last = max(last, m.Page)
				if m.Page != page {
					continue
				}
				supported := []map[string]any{}
				for _, e := range m.Efforts {
					supported = append(supported, map[string]any{"reasoningEffort": e})
				}
				row := map[string]any{
					"id": m.Value, "model": m.resolved(), "hidden": m.Hidden,
					"defaultReasoningEffort": m.DefaultEffort, "supportedReasoningEfforts": supported,
				}
				if m.Upgrade != "" {
					row["upgrade"] = m.Upgrade
				}
				rows = append(rows, row)
			}
			result := map[string]any{"data": rows}
			if page < last {
				result["nextCursor"] = "page-" + strconv.Itoa(page+1)
			}
			err = out.Encode(map[string]any{"id": *req.ID, "result": result})
		case "skills/list":
			fail, werr := cfg.wait()
			if werr != nil {
				return 0, werr
			}
			if fail {
				return 0, errors.New("fakecodex probe: configured to fail")
			}
			skills := make([]map[string]any, 0, len(cfg.Entries))
			for _, e := range cfg.Entries {
				skills = append(skills, map[string]any{
					"name": e.Name, "description": e.Description,
					"path":  filepath.Join(cwd, ".agents", "skills", e.Name, "SKILL.md"),
					"scope": "user", "enabled": !e.Disabled, "pluginId": nil,
				})
			}
			err = out.Encode(map[string]any{"id": *req.ID, "result": map[string]any{
				"data": []any{map[string]any{"cwd": cwd, "skills": skills, "errors": []any{}}}}})
			if err == nil {
				return 0, nil
			}
		}
		if err != nil {
			return 0, err
		}
	}
	return 0, sc.Err()
}
