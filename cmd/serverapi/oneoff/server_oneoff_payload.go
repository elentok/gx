package oneoff

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
)

// oneOffFileFrontmatter is the subset of a payload file's frontmatter a submit
// understands.
type oneOffFileFrontmatter struct {
	Project               string   `yaml:"project"`
	Name                  string   `yaml:"name"`
	Agent                 string   `yaml:"agent"`
	BlockedBy             []string `yaml:"blocked_by"`
	ExpectedContextWindow int      `yaml:"expected_context_window"`
	Front                 bool     `yaml:"front"`
	Unique                string   `yaml:"unique"`
}

// removedPayloadKeys are payload keys one-offs no longer accept.
var removedPayloadKeys = []string{"commits", "base"}

// applyOneOffPayload merges the payload file at path into req: anything already
// set on req wins, the body is the prompt when req has none, and the dedupe key
// defaults to the file's resolved path unless req opts out. A file that cannot
// be used is a refusal, shaped like the server's so --json and exit codes match.
func applyOneOffPayload(req *wire.OneOffRequest, path string) *wire.OneOffResult {
	refuse := func(reason string, err error) *wire.OneOffResult {
		return &wire.OneOffResult{Refused: true, Reason: reason, Message: err.Error()}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return refuse(wire.ReasonBadFile, err)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return refuse(wire.ReasonBadFile, err)
	}
	yamlPart, body, hasFM := schema.SplitFrontmatter(string(raw))
	var fm oneOffFileFrontmatter
	if hasFM {
		if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
			return refuse(wire.ReasonBadFile, err)
		}
		var keys map[string]any
		if err := yaml.Unmarshal([]byte(yamlPart), &keys); err != nil {
			return refuse(wire.ReasonBadFile, err)
		}
		for _, k := range removedPayloadKeys {
			if _, ok := keys[k]; ok {
				return refuse(wire.ReasonOptionRemoved, fmt.Errorf("the %s key is no longer supported: a one-off never commits", k))
			}
		}
	} else {
		body = string(raw)
	}
	setIfEmpty := func(dst *string, v string) {
		if *dst == "" {
			*dst = v
		}
	}
	setIfEmpty(&req.Prompt, body)
	setIfEmpty(&req.Project, fm.Project)
	setIfEmpty(&req.Name, fm.Name)
	setIfEmpty(&req.Agent, fm.Agent)
	setIfEmpty(&req.Unique, fm.Unique)
	req.Front = req.Front || fm.Front
	if len(req.BlockedBy) == 0 {
		req.BlockedBy = fm.BlockedBy
	}
	if req.ExpectedContextWindow == 0 {
		req.ExpectedContextWindow = fm.ExpectedContextWindow
	}
	// The key comes after the frontmatter merge: the file may set `unique`.
	if req.Unique == "" && !req.NoUnique {
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return refuse(wire.ReasonBadFile, err)
		}
		req.Unique = resolved
	}
	return nil
}
