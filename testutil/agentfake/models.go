package agentfake

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ModelQueryLogEnv names a file every model query appends one JSON line to
// ({"harness":"claude","at":...}). Unset, queries are not logged.
const ModelQueryLogEnv = "GX_AGENTFAKE_MODEL_QUERY_LOG"

// What a launched model does, set per model in ProbeModel.Behaviour.
const (
	// ModelExitAtStart makes the launch exit before it reports idle.
	ModelExitAtStart = "exit-at-start"
	// ModelHold keeps the launch from going idle until its release file exists.
	ModelHold = "hold"
	// ModelNotFound ends the first message in a model_not_found error.
	ModelNotFound = "model-not-found"
	// ModelError ends the first message in the error code ProbeModel.ErrorCode
	// names, one that must not roll a switch back.
	ModelError = "error"
)

// ProbeModel is one row of a fake's model list. A Claude row uses Value, ID
// and Efforts; a Codex row also Hidden, DefaultEffort, Upgrade and Page.
type ProbeModel struct {
	// Value is what a launch takes (Claude value, Codex id).
	Value string `json:"value"`
	// ID is the resolved model id (Claude resolvedModel, Codex model).
	ID      string   `json:"id,omitempty"`
	Efforts []string `json:"efforts,omitempty"`
	// DefaultEffort, Hidden and Upgrade are Codex-only.
	DefaultEffort string `json:"defaultEffort,omitempty"`
	Hidden        bool   `json:"hidden,omitempty"`
	Upgrade       string `json:"upgrade,omitempty"`
	// Page is the Codex model/list page (0-based) the row comes on.
	Page int `json:"page,omitempty"`
	// Behaviour is one of the Model... consts; empty is a normal launch.
	Behaviour string `json:"behaviour,omitempty"`
	// ErrorCode is the error a ModelError launch answers with.
	ErrorCode string `json:"errorCode,omitempty"`
}

func (m ProbeModel) resolved() string {
	if m.ID != "" {
		return m.ID
	}
	return m.Value
}

// LaunchModel finds the configured row a launch's --model names, by value or
// by resolved id. ok is false for a model the list does not have.
func LaunchModel(model string) (ProbeModel, bool) {
	if model == "" {
		return ProbeModel{}, false
	}
	cfg, err := loadProbeConfig()
	if err != nil {
		return ProbeModel{}, false
	}
	for _, m := range cfg.Models {
		if m.Value == model || m.ID == model {
			return m, true
		}
	}
	return ProbeModel{}, false
}

// ResolveClaudeModel is the id a Claude launched with model reports: the
// resolved id of an alias in the list, model itself otherwise.
func ResolveClaudeModel(model string) string {
	if m, ok := LaunchModel(model); ok {
		return m.resolved()
	}
	return model
}

// DefaultClaudeModels is the Claude list the model-switch flows use: the
// default alias, a lagging alias, a model with no effort control, a 4.6 model
// without xhigh, an alias of another family and an unparseable id.
func DefaultClaudeModels() []ProbeModel {
	all := []string{"low", "medium", "high", "xhigh"}
	return []ProbeModel{
		{Value: "default", ID: "claude-sonnet-5-5", Efforts: all},
		{Value: "fable", ID: "claude-fable-5-1", Efforts: all},
		{Value: "claude-fable-5-2", ID: "claude-fable-5-2", Efforts: all},
		{Value: "haiku", ID: "claude-haiku-5-5"},
		{Value: "claude-opus-4-6", ID: "claude-opus-4-6", Efforts: []string{"low", "medium", "high"}},
		{Value: "opus", ID: "claude-opus-5-5", Efforts: all},
		{Value: "mystery", ID: "mystery-model"},
	}
}

// DefaultCodexModels is the Codex list the model-switch flows use: two
// versions of a family where upgrade names the newer, a family with no
// successor, a hidden model, an unparseable id and a second page.
func DefaultCodexModels() []ProbeModel {
	efforts := []string{"low", "medium", "high"}
	return []ProbeModel{
		{Value: "gpt-5.4", ID: "gpt-5.4", Efforts: efforts, DefaultEffort: "medium", Upgrade: "gpt-5.5"},
		{Value: "gpt-5.5", ID: "gpt-5.5", Efforts: efforts, DefaultEffort: "medium"},
		{Value: "gpt-6-mini", ID: "gpt-6-mini", Efforts: efforts, DefaultEffort: "low"},
		{Value: "gpt-5.3-hidden", ID: "gpt-5.3-hidden", Efforts: efforts, DefaultEffort: "medium", Hidden: true},
		{Value: "mystery", ID: "mystery-model", Efforts: efforts, DefaultEffort: "medium"},
		{Value: "gpt-5.6", ID: "gpt-5.6", Efforts: efforts, DefaultEffort: "medium", Page: 1},
	}
}

// LogModelQuery appends one line for a model query by harness.
func LogModelQuery(harness string) error {
	path := os.Getenv(ModelQueryLogEnv)
	if path == "" {
		return nil
	}
	return appendJSONLine(path, map[string]any{"harness": harness, "at": time.Now().UTC().Format(time.RFC3339Nano)})
}

// ReadModelQueries is the harness of each model query in the log, in order.
func ReadModelQueries(path string) ([]string, error) {
	var out []string
	err := readJSONLines(path, func(raw []byte) error {
		var q struct {
			Harness string `json:"harness"`
		}
		if err := json.Unmarshal(raw, &q); err != nil {
			return err
		}
		out = append(out, q.Harness)
		return nil
	})
	return out, err
}

// AppendLaunch appends one launch's argv to the launch log at path. The log
// keeps every launch of a conversation, where the .args file keeps the last.
func AppendLaunch(path string, args []string) error {
	return appendJSONLine(path, args)
}

// ReadLaunches is the argv of each launch in the log at path, in order.
func ReadLaunches(path string) ([][]string, error) {
	var out [][]string
	err := readJSONLines(path, func(raw []byte) error {
		var args []string
		if err := json.Unmarshal(raw, &args); err != nil {
			return err
		}
		out = append(out, args)
		return nil
	})
	return out, err
}

// WaitForRelease blocks until the file at path exists.
func WaitForRelease(path string, timeout time.Duration) error {
	for deadline := time.Now().Add(timeout); ; time.Sleep(10 * time.Millisecond) {
		if FileExists(path) {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("agentfake: launch never released: " + path)
		}
	}
}

// HoldTimeout bounds how long a held launch waits for its release file.
const HoldTimeout = 2 * time.Minute

func appendJSONLine(path string, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

func readJSONLines(path string, each func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		if err := each(sc.Bytes()); err != nil {
			return err
		}
	}
	return sc.Err()
}
