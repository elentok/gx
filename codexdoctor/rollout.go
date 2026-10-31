package codexdoctor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// checkRolloutFormat runs CheckRollout over every rollout in the recording and
// names the first missing field.
func checkRolloutFormat(in Input) (string, bool) {
	if len(in.Rollouts) == 0 {
		return "no rollouts to check", false
	}
	for _, r := range in.Rollouts {
		if problem := CheckRollout(r.Data); problem != "" {
			return fmt.Sprintf("%s: %s", r.Name, problem), false
		}
	}
	return fmt.Sprintf("%d rollouts have every field the readers use", len(in.Rollouts)), true
}

// CheckRollout verifies each field gx's readers use in one rollout and returns
// the first problem, or "" when the rollout is fine. A kind of line the rollout
// does not hold is not a problem. Fields are checked as raw JSON paths, not
// through the reader structs, because a struct cannot tell a missing field
// from a zero one.
func CheckRollout(data []byte) string {
	for i, raw := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal(raw, &line); err != nil {
			return fmt.Sprintf("line %d: not JSON: %v", i+1, err)
		}
		if path := missingField(line); path != "" {
			return fmt.Sprintf("%s missing (line %d)", path, i+1)
		}
	}
	return ""
}

// missingField returns the first required path the line lacks.
func missingField(line map[string]any) string {
	var required []string
	switch str(line, "type") {
	case "session_meta":
		required = []string{"payload.id", "payload.cwd", "payload.cli_version"}
	case "compacted":
		required = []string{"payload"}
	case "response_item":
		switch str(line, "payload.type") {
		case "message":
			required = []string{"payload.role", "payload.content"}
		case "function_call":
			required = []string{"payload.name", "payload.call_id", "payload.arguments"}
		case "custom_tool_call":
			required = []string{"payload.name", "payload.call_id", "payload.input"}
		}
	case "event_msg":
		switch str(line, "payload.type") {
		case "task_complete":
			required = []string{"payload.turn_id"}
			if _, ok := lookup(line, "payload.error"); ok {
				required = append(required, "payload.error.message", "payload.error.codex_error_info")
			}
		case "token_count":
			required = []string{
				"payload.info.total_token_usage.total_tokens",
				"payload.rate_limits.primary.used_percent",
				"payload.rate_limits.primary.resets_at",
			}
		}
	}
	for _, p := range required {
		if _, ok := lookup(line, p); !ok {
			return p
		}
	}
	return ""
}

// lookup walks a dotted path; a null value counts as absent.
func lookup(m map[string]any, path string) (any, bool) {
	var cur any = m
	for _, key := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = obj[key]; !ok {
			return nil, false
		}
	}
	return cur, cur != nil
}

func str(m map[string]any, path string) string {
	v, _ := lookup(m, path)
	s, _ := v.(string)
	return s
}
