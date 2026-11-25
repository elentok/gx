package bgtx

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// Names of the Codex shapes of the subagents scenario. Each parent session has
// one spawn_agent call (Output.Subagents.Calls["spawn"]) whose child thread id
// is Calls["thread"].
const (
	CodexSubName      = "review"
	CodexSubMarker    = "You are now a newly spawned child agent. The preceding conversation is inherited parent context."
	CodexSubLast      = "The change looks fine."
	CodexSubGrandcall = "cxgc1"

	// CodexSubVersion is the parent rollout's cli_version, the one the bg-progress
	// codex shape shows its subagent row with.
	CodexSubVersion = "0.166.0"

	// CodexSubRolloutKey is the child rollout's key in Output.TaskFiles: written at
	// start, except in the starting shape where a flow writes it.
	CodexSubRolloutKey = "subagent-codex-rollout"
)

// codexChildKind is what a Codex shape leaves on disk for its child.
type codexChildKind int

const (
	codexNoRollout   codexChildKind = iota // the call and "started" event only
	codexChild                             // a rollout with a role
	codexNullRole                          // a rollout with agent_role null
	codexCompressed                        // only a .jsonl.zst
	codexNestedChild                       // a rollout whose own lines spawn a grandchild
)

// CodexSubagentShapes are the Codex shapes of the subagents scenario, by --shape.
var CodexSubagentShapes = map[string]Subagent{
	"codex-starting":   codexShape("codex-starting", codexNoRollout),
	"codex-child":      codexShape("codex-child", codexChild),
	"codex-null-role":  codexShape("codex-null-role", codexNullRole),
	"codex-compressed": codexShape("codex-compressed", codexCompressed),
	"codex-nested":     codexShape("codex-nested", codexNestedChild),
}

var codexThreads = map[string]string{"codex-starting": "e1", "codex-child": "e2", "codex-null-role": "e3", "codex-compressed": "e4", "codex-nested": "e5"}

// CodexSubThread is the child thread id of a Codex shape.
func CodexSubThread(shape string) string {
	return "00000000-0000-4000-8000-0000000000" + codexThreads[shape]
}

func codexShape(shape string, kind codexChildKind) Subagent {
	thread, call := CodexSubThread(shape), "cxsp-"+shape
	return Subagent{
		Codex: true,
		Build: func(b *Lines) { b.codexSubChild(3*minute, call, thread, kind) },
		Calls: map[string]string{"spawn": call, "thread": thread},
	}
}

// codexSubChild is the parent's spawn_agent call and what the kind says of the
// child's rollout.
func (b *Lines) codexSubChild(ago time.Duration, call, thread string, kind codexChildKind) {
	b.codexSpawn(ago, call, CodexSubName, thread, true)
	name := "rollout-2026-01-01T00-00-00-" + thread
	path := b.p.Rollouts + "/" + name + ".jsonl"
	switch kind {
	case codexNoRollout:
		b.reserve(File{Path: path, Key: CodexSubRolloutKey})
		return
	}
	content := strings.Join(b.codexChildLines(ago, thread, kind), "\n") + "\n"
	if kind == codexCompressed {
		enc, err := zstd.NewWriter(nil)
		if err != nil {
			panic(err)
		}
		path, content = b.p.Rollouts+"/"+name+".jsonl.zst", string(enc.EncodeAll([]byte(content), nil))
	}
	b.file(File{Path: path, Content: content, Key: CodexSubRolloutKey})
}

// codexChildLines are the child rollout: the session_meta, the parent's history
// ending in the marker, and from subagent_history_start_ordinal the child's own
// lines.
func (b *Lines) codexChildLines(ago time.Duration, thread string, kind codexChildKind) []string {
	at := func(d time.Duration) string { return b.when(d).UTC().Format(time.RFC3339Nano) }
	msg := func(role, text string) string {
		return jsonLine(map[string]any{"timestamp": at(ago), "type": "response_item", "payload": map[string]any{
			"type": "message", "role": role, "content": []any{map[string]any{"type": "output_text", "text": text}}}})
	}
	history := []string{msg("user", "Please review the change."), msg("assistant", "I will ask a subagent."), msg("developer", CodexSubMarker)}
	ordinal := 1 + len(history) // session_meta is line 0
	var role any = "reviewer"
	if kind == codexNullRole {
		role = nil
	}
	lines := []string{jsonLine(map[string]any{"timestamp": at(ago), "type": "session_meta", "payload": map[string]any{
		"id": thread, "cli_version": CodexSubVersion, "agent_role": role, "agent_path": "/root/" + CodexSubName,
		"thread_source": "subagent", "subagent_history_start_ordinal": ordinal}})}
	lines = append(lines, history...)
	lines = append(lines,
		jsonLine(map[string]any{"timestamp": at(ago), "type": "event_msg", "payload": map[string]any{"type": "agent_message", "encrypted_content": "gAAAAABfake"}}),
		jsonLine(map[string]any{"type": "turn_context", "payload": map[string]any{"model": CodexChildModel, "effort": "medium"}}),
		jsonLine(map[string]any{"timestamp": at(ago), "type": "response_item", "payload": map[string]any{
			"type": "function_call", "name": "exec_command", "call_id": "cxk1", "arguments": jsonLine(map[string]any{"cmd": "git diff"})}}),
		jsonLine(map[string]any{"timestamp": at(ago), "type": "response_item", "payload": map[string]any{
			"type": "function_call_output", "call_id": "cxk1", "output": "diff --git a/x b/x"}}),
	)
	if kind == codexNestedChild {
		lines = append(lines, jsonLine(map[string]any{"timestamp": at(ago), "type": "response_item", "payload": map[string]any{
			"type": "function_call", "name": "spawn_agent", "call_id": CodexSubGrandcall,
			"arguments": jsonLine(map[string]any{"task_name": "grandchild", "message": "Read the wire package."})}}))
	}
	lines = append(lines,
		jsonLine(map[string]any{"type": "event_msg", "payload": map[string]any{"type": "token_count", "info": map[string]any{
			"total_token_usage":    map[string]any{"total_tokens": CodexChildTokens},
			"last_token_usage":     map[string]any{"input_tokens": 1000, "total_tokens": CodexChildTokens},
			"model_context_window": 200000,
		}}}),
		msg("assistant", CodexSubLast),
	)
	// The chat read finds the child's own lines by the ordinal each line carries.
	for i, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			panic(err)
		}
		m["ordinal"] = i
		lines[i] = jsonLine(m)
	}
	return lines
}
