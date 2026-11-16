package spec

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// BgPaths are the folders a bg-progress seed writes into: the tasks folder
// holds shell output files and subagent symlinks, Subagents the subagent files,
// Rollouts the Codex home's sessions folder (child rollouts go there).
type BgPaths struct{ Tasks, Subagents, Rollouts string }

// BgFile is one file the seeder writes. A Link makes Path a symlink to Link.
// Key records Path in Output.TaskFiles under that name.
type BgFile struct {
	Path, Content, Link, Key string
	Mtime                    time.Time
}

// BgSeed is what a bg-progress session gets after the fake wrote its own
// transcript: Lines are appended to it and Files are written beside it.
type BgSeed struct {
	Lines []string
	Files []BgFile
	// Version, when set, is written as the Codex rollout's cli_version.
	Version string
	// Age is how long before the seed the session was created; zero is the
	// seeder's default. A task that starts before the run began reads as one of
	// an earlier run, so a seed with old tasks asks for a longer age.
	Age time.Duration
	// Working makes the seeder also wait for the chat read's Working row.
	Working bool
}

type bgEntry struct {
	at   time.Time
	line string
}

// bgLines builds the parent transcript lines and files of one session. All
// times are offsets before the seed time, truncated to the second.
type bgLines struct {
	now     time.Time
	p       BgPaths
	entries []bgEntry
	files   []BgFile
}

func newBgLines(now time.Time, p BgPaths) *bgLines {
	return &bgLines{now: now.Truncate(time.Second), p: p}
}

func (b *bgLines) seed() BgSeed {
	slices.SortStableFunc(b.entries, func(x, y bgEntry) int { return x.at.Compare(y.at) })
	out := BgSeed{Files: b.files}
	for _, e := range b.entries {
		out.Lines = append(out.Lines, e.line)
	}
	return out
}

func (b *bgLines) when(ago time.Duration) time.Time { return b.now.Add(-ago) }

func jsonLine(m map[string]any) string {
	raw, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func (b *bgLines) add(ago time.Duration, m map[string]any) {
	at := b.when(ago)
	m["timestamp"] = at.UTC().Format(time.RFC3339Nano)
	m["uuid"] = fmt.Sprintf("bg-%03d", len(b.entries))
	b.entries = append(b.entries, bgEntry{at, jsonLine(m)})
}

// use is an assistant line with one tool call.
func (b *bgLines) use(ago time.Duration, id, name string, input map[string]any) {
	b.add(ago, map[string]any{"type": "assistant", "message": map[string]any{
		"id": "msg_" + id, "role": "assistant",
		"content": []any{map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}},
	}})
}

// result is the user line answering a tool call; res is its toolUseResult.
func (b *bgLines) result(ago time.Duration, id, text string, res map[string]any) {
	m := map[string]any{"type": "user", "message": map[string]any{
		"role":    "user",
		"content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "content": text}},
	}}
	if res != nil {
		m["toolUseResult"] = res
	}
	b.add(ago, m)
}

func (b *bgLines) notify(ago time.Duration, body string) {
	b.add(ago, map[string]any{"type": "queue-operation", "operation": "enqueue", "content": "<task-notification>\n" + body + "\n</task-notification>"})
}

func (b *bgLines) outputPath(taskID string) string { return b.p.Tasks + "/" + taskID + ".output" }

func (b *bgLines) file(f BgFile) { b.files = append(b.files, f) }

// outputText is n lines of about width bytes each.
func outputText(label string, n, width int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		line := fmt.Sprintf("%s line %02d ", label, i)
		sb.WriteString(line + strings.Repeat(".", max(width-len(line)-1, 0)) + "\n")
	}
	return sb.String()
}

// shell launches a background shell at ago and writes its output file. The
// launch result lands a second later and names the file.
func (b *bgLines) shell(ago time.Duration, useID, taskID, command, output string, mtime time.Duration) {
	b.use(ago, useID, "Bash", map[string]any{"command": command, "run_in_background": true})
	path := b.outputPath(taskID)
	b.result(ago-time.Second, useID, "Command running in background with ID: "+taskID+". Output is being written to: "+path,
		map[string]any{"backgroundTaskId": taskID})
	b.file(BgFile{Path: path, Content: output, Mtime: b.when(mtime)})
}

// endShell is the final notification of a shell.
func (b *bgLines) endShell(ago time.Duration, taskID, status, summary string) {
	b.notify(ago, fmt.Sprintf("<task-id>%s</task-id>\n<output-file>%s</output-file>\n<status>%s</status>\n<summary>%s</summary>",
		taskID, b.outputPath(taskID), status, summary))
}

// BgAgentID is the id of the subagent most shapes launch first.
const BgAgentID = "a5c0de01"

// subagent launches an async subagent at ago; its file holds fileLines and its
// symlink sits in the tasks folder, as Claude Code writes them.
func (b *bgLines) subagent(ago time.Duration, id, desc string, fileLines []string) {
	useID := "toolu_agent_" + id
	b.use(ago, useID, "Agent", map[string]any{"description": desc, "subagent_type": "Explore", "prompt": desc + " and report back."})
	file := b.p.Subagents + "/agent-" + id + ".jsonl"
	link := b.outputPath(id)
	b.result(ago-time.Second, useID, "Async agent launched successfully.\nagentId: "+id,
		map[string]any{"status": "async_launched", "agentId": id, "resolvedModel": bgModel, "outputFile": link})
	b.file(BgFile{Path: file, Content: strings.Join(fileLines, "\n") + "\n", Mtime: b.when(ago - time.Second)})
	b.file(BgFile{Path: link, Link: file})
}

const bgModel = "claude-sonnet-5-5"

// subLine is one line of a subagent's own file; every one is a sidechain line.
func subLine(m map[string]any) string {
	m["isSidechain"] = true
	return jsonLine(m)
}

func subAssistant(msgID string, in, out int, blocks ...any) string {
	return subLine(map[string]any{"type": "assistant", "message": map[string]any{
		"id": msgID, "model": bgModel, "role": "assistant", "content": blocks,
		"usage": map[string]any{"input_tokens": in, "output_tokens": out},
	}})
}

func subUse(id, name string, input map[string]any) any {
	return map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}
}

func subResult(id, text string, res map[string]any) string {
	m := map[string]any{"type": "user", "message": map[string]any{"role": "user",
		"content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "content": text}}}}
	if res != nil {
		m["toolUseResult"] = res
	}
	return subLine(m)
}

// The tokens and tool calls of the survey subagent's file below.
const (
	BgSurveyTokens    = 4720
	BgSurveyToolCalls = 3
	BgSurveyCurrent   = "InFlightTool"
)

// surveyFile has usage on three message ids (one repeated by two parallel
// calls), three tool calls and an open Grep.
func surveyFile() []string {
	return []string{
		subAssistant("msg_s1", 1200, 40, map[string]any{"type": "text", "text": "Listing the sessions code."}),
		subAssistant("msg_s2", 1500, 60, subUse("toolu_s1", "Read", map[string]any{"file_path": "server/sessions/sessions.go"})),
		subResult("toolu_s1", "package sessions", nil),
		subAssistant("msg_s3", 1800, 120, subUse("toolu_s2", "Glob", map[string]any{"pattern": "server/sessions/*.go"})),
		subAssistant("msg_s3", 1800, 120, subUse("toolu_s3", "Grep", map[string]any{"pattern": BgSurveyCurrent})),
		subResult("toolu_s2", "server/sessions/sessions.go", nil),
	}
}

func bashScript(lines int) string {
	out := []string{"#!/usr/bin/env bash", "set -euo pipefail"}
	for i := len(out); i < lines; i++ {
		out = append(out, fmt.Sprintf("echo \"step %d\" && sleep 1", i-1))
	}
	return strings.Join(out, "\n")
}
