package spec

import (
	"encoding/json"
	"strings"
	"time"
)

// The Codex subagent's child rollout, which the codex shape's chat read shows.
const (
	BgCodexChildModel  = "gpt-5.5-codex"
	BgCodexChildTokens = 3210
)

// bgOldCodexCommand is the command of the old-codex shape's open call, which is
// also the Working row's label.
const bgOldCodexCommand = "make old"

// BgOldCodexLabel is the Working row's label of the old-codex shape.
const BgOldCodexLabel = bgOldCodexCommand

// rollout is one Codex rollout line at ago.
func (b *bgLines) rollout(ago time.Duration, typ string, payload map[string]any) {
	at := b.when(ago)
	b.entries = append(b.entries, bgEntry{at, jsonLine(map[string]any{
		"timestamp": at.UTC().Format(time.RFC3339Nano), "type": typ, "payload": payload,
	})})
}

// cell is the exec call of a script that runs one command.
func (b *bgLines) cell(ago time.Duration, callID, command string) {
	lit, _ := json.Marshal(command)
	b.rollout(ago, "response_item", map[string]any{
		"type": "custom_tool_call", "name": "exec", "call_id": callID,
		"input": "const r = await tools.exec_command({cmd:" + string(lit) + "}); text(JSON.stringify(r));",
	})
}

// cellOut is the output of an exec call: a JSON string holding input_text items.
func (b *bgLines) cellOut(ago time.Duration, callID string, texts ...string) {
	var items []map[string]string
	for _, s := range texts {
		items = append(items, map[string]string{"type": "input_text", "text": s})
	}
	raw, _ := json.Marshal(items)
	b.rollout(ago, "response_item", map[string]any{"type": "custom_tool_call_output", "call_id": callID, "output": string(raw)})
}

func (b *bgLines) fnCall(ago time.Duration, callID, name, arguments string) {
	b.rollout(ago, "response_item", map[string]any{"type": "function_call", "name": name, "call_id": callID, "arguments": arguments})
}

func (b *bgLines) fnOut(ago time.Duration, callID, text string) {
	b.rollout(ago, "response_item", map[string]any{"type": "function_call_output", "call_id": callID, "output": text})
}

// commandDone is the item_completed line of a command (Codex 0.147.0 and later).
func (b *bgLines) commandDone(ago, started time.Duration, command string, code int, output string) {
	b.rollout(ago, "event_msg", map[string]any{
		"type": "item_completed",
		"item": map[string]any{
			"type": "CommandExecution", "id": "exec-1", "process_id": "4001", "status": "completed",
			"command": []string{"/bin/zsh", "-lc", command}, "exit_code": code, "aggregated_output": output,
		},
		"started_at_ms":   b.when(started).UnixMilli(),
		"completed_at_ms": b.when(ago).UnixMilli(),
	})
}

// codexSubagent spawns a subagent whose child rollout (thread) the seeder writes.
func (b *bgLines) codexSubagent(ago time.Duration, callID, name, thread string) {
	b.fnCall(ago, callID, "spawn_agent", jsonLine(map[string]any{"task_name": name, "message": "Review the change."}))
	b.fnOut(ago-time.Second, callID, jsonLine(map[string]any{"task_name": "/root/" + name}))
	b.rollout(ago-2*time.Second, "event_msg", map[string]any{
		"type": "item_completed",
		"item": map[string]any{
			"type": "SubAgentActivity", "kind": "started", "id": callID,
			"agent_thread_id": thread, "agent_path": "/root/" + name, "model": "gpt-spawn",
		},
	})
	child := []string{
		jsonLine(map[string]any{"timestamp": b.when(ago).UTC().Format(time.RFC3339Nano), "type": "session_meta", "payload": map[string]any{"id": thread, "cli_version": "0.166.0"}}),
		jsonLine(map[string]any{"type": "turn_context", "payload": map[string]any{"model": BgCodexChildModel, "effort": "medium"}}),
		jsonLine(map[string]any{"type": "event_msg", "payload": map[string]any{"type": "token_count", "info": map[string]any{
			"total_token_usage":    map[string]any{"total_tokens": BgCodexChildTokens},
			"last_token_usage":     map[string]any{"input_tokens": 1000, "total_tokens": BgCodexChildTokens},
			"model_context_window": 200000,
		}}}),
	}
	b.file(BgFile{
		Path:    b.p.Rollouts + "/rollout-2026-01-01T00-00-00-" + thread + ".jsonl",
		Content: strings.Join(child, "\n") + "\n",
	})
}

// turnEnd is the line that ends the foreground turn.
func (b *bgLines) turnEnd(ago time.Duration) {
	b.add(ago, map[string]any{"type": "system", "subtype": "turn_duration"})
}

// bgCodex: a done command, a yielded command a wait polls, and a subagent.
// withItems adds the done command's item_completed line (Codex 0.147.0 and
// later); without it the command ends through its Script completed output.
func bgCodex(withItems bool, thread string) func(b *bgLines) {
	return func(b *bgLines) {
		b.cell(5*minute, "cx1", "make build")
		if withItems {
			b.cellOut(5*minute-2*sec, "cx1", "Script running with cell ID 7")
			b.commandDone(2*minute, 5*minute-2*sec, "make build", 0, "built\nok\n")
			b.fnCall(2*minute-sec, "cxw0", "wait", `{"cell_id":7}`)
			b.fnOut(2*minute-2*sec, "cxw0", "Script completed\nbuilt")
		} else {
			b.cellOut(2*minute, "cx1", "Script completed", `{"output":"ok"}`)
		}
		b.cell(4*minute, "cx2", "make test-long")
		b.cellOut(4*minute-2*sec, "cx2", "Script running with cell ID 8")
		b.fnCall(90*sec, "cxw1", "wait", `{"cell_id":8}`)
		b.codexSubagent(3*minute, "cxsp1", "review", thread)
	}
}

// bgOldCodex: the old shape, an open exec_command function call.
func bgOldCodex(b *bgLines) {
	b.fnCall(minute, "old1", "exec_command", jsonLine(map[string]any{"cmd": bgOldCodexCommand}))
}

// bgExitedClaude: a running shell, Monitor and subagent of a crashed session.
func bgExitedClaude(b *bgLines) {
	b.subagent(4*minute, BgAgentID, "Survey the sessions code", surveyFile())
	b.shell(5*minute, "toolu_watch", "bwatch001", "make watch", outputText("watch", 3, 30), 20*sec)
	b.use(3*minute, "toolu_monitor", "Monitor", map[string]any{"command": "tail -f logs/app.log", "timeout_ms": 600000, "description": "Watch the app log"})
	b.result(3*minute-sec, "toolu_monitor", "Monitor started: bmon00001", map[string]any{"taskId": "bmon00001", "timeoutMs": 600000})
}

// bgExitedCodex: a yielded cell of a crashed session that never ended.
func bgExitedCodex(b *bgLines) {
	b.cell(3*minute, "cx1", "make watch")
	b.cellOut(3*minute-2*sec, "cx1", "Script running with cell ID 5")
}

// The Monitor of the no-end shape: its timeout and event notices.
const (
	BgNoEndMonitorTimeout = 20 * time.Second
	BgNoEndAge            = 4 * time.Hour
)

// bgNoEndClaude: a foreground call with no result and a later turn end, a shell
// past the age-out, a Monitor that expires, a shell with an exit trailer and no
// notification, and one running shell that keeps the group open.
func bgNoEndClaude(b *bgLines) {
	b.shell(3*time.Hour, "toolu_old", "bold00001", "make old", "old output\n", 3*time.Hour)
	b.shell(90*minute, "toolu_keep", "bkeep0001", "make watch", outputText("watch", 3, 30), 20*sec)

	b.use(25*minute, "toolu_fg", "Bash", map[string]any{"command": "make verify"})
	b.turnEnd(15 * minute)

	b.shell(8*minute, "toolu_trail", "btrail001", "make flaky", "flaky line\n[exited with code 3]\n", 2*minute)

	b.use(5*sec, "toolu_mon", "Monitor", map[string]any{"command": "tail -f logs/app.log", "timeout_ms": 20000, "description": "Watch the app log"})
	b.result(4*sec, "toolu_mon", "Monitor started: bmon00001", map[string]any{"taskId": "bmon00001", "timeoutMs": BgNoEndMonitorTimeout.Milliseconds()})
	b.notify(3*sec, "<task-id>bmon00001</task-id>\n<summary>Monitor event: hi</summary>\n<event>hi</event>")
	// The expiry notice lands after the timeout.
	b.notify(-16*sec, "<task-id>bmon00001</task-id>\n<summary>Monitor expired</summary>\n<event>[Monitor expired after 1m with 1 event delivered. Re-arm it if you still need the watch.]</event>")
	b.file(BgFile{Path: b.outputPath("bmon00001"), Content: "hi\n\n[killed]\n", Mtime: b.when(3 * sec)})
}

// bgNoEndCodex: a yielded cell with no end.
func bgNoEndCodex(b *bgLines) {
	b.cell(2*minute, "cx1", "make serve")
	b.cellOut(2*minute-2*sec, "cx1", "Script running with cell ID 3")
}
