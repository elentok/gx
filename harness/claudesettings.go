package harness

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
)

// EditorModeNormal is the editor mode every gx-launched claude starts in, so a
// person's vim mode never leaves the input in NORMAL where typed text is lost.
const EditorModeNormal = "normal"

const settingsFlag = "--settings"

// MergeClaudeSettings folds every --settings flag in args, plus editorMode
// normal, into one --settings flag at the first one's place (or appended).
// Claude may let a second flag replace the first, so one flag carries it all.
// A value is inline JSON or a path to a JSON file; an unreadable one is
// dropped. Merging is idempotent: objects merge key by key, arrays concatenate
// without repeating an element.
func MergeClaudeSettings(args []string) []string {
	merged := map[string]any{}
	out := make([]string, 0, len(args)+2)
	at := -1
	for i := 0; i < len(args); i++ {
		value, ok := "", false
		switch {
		case args[i] == settingsFlag && i+1 < len(args):
			i++
			value, ok = args[i], true
		case strings.HasPrefix(args[i], settingsFlag+"="):
			value, ok = strings.TrimPrefix(args[i], settingsFlag+"="), true
		}
		if !ok {
			out = append(out, args[i])
			continue
		}
		if at < 0 {
			at = len(out)
		}
		if m := readSettings(value); m != nil {
			mergeInto(merged, m)
		}
	}
	merged["editorMode"] = EditorModeNormal
	data, _ := json.Marshal(merged)
	flag := []string{settingsFlag, string(data)}
	if at < 0 {
		return append(out, flag...)
	}
	return slices.Insert(out, at, flag...)
}

// NoteLaunchedArgs is for a process that runs a launch command another process
// built (the pty holder): the editor-mode override lives in the args, but the
// probe's memory of it is per process, so the runner of the command records it.
// env is the claude config env the probe reads, nil for the process's own.
func NoteLaunchedArgs(session string, args, env []string) {
	normal := false
	for i := 0; i < len(args); i++ {
		value := ""
		switch {
		case args[i] == settingsFlag && i+1 < len(args):
			i++
			value = args[i]
		case strings.HasPrefix(args[i], settingsFlag+"="):
			value = strings.TrimPrefix(args[i], settingsFlag+"=")
		default:
			continue
		}
		if m := readSettings(value); m != nil {
			normal = m["editorMode"] == EditorModeNormal
		}
	}
	if normal {
		markEditorOverride(session, env)
	}
}

func readSettings(value string) map[string]any {
	raw := strings.TrimSpace(value)
	if !strings.HasPrefix(raw, "{") {
		data, err := os.ReadFile(raw)
		if err != nil {
			return nil
		}
		raw = string(data)
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return nil
	}
	return m
}

func mergeInto(dst, src map[string]any) {
	for k, v := range src {
		switch cur := dst[k].(type) {
		case map[string]any:
			if vm, ok := v.(map[string]any); ok {
				mergeInto(cur, vm)
				continue
			}
		case []any:
			if va, ok := v.([]any); ok {
				for _, e := range va {
					if !containsJSON(cur, e) {
						cur = append(cur, e)
					}
				}
				dst[k] = cur
				continue
			}
		}
		dst[k] = v
	}
}

func containsJSON(list []any, e any) bool {
	want, _ := json.Marshal(e)
	return slices.ContainsFunc(list, func(x any) bool {
		got, _ := json.Marshal(x)
		return string(got) == string(want)
	})
}
