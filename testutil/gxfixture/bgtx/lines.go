// Package bgtx builds the transcripts and task files of the bg-progress
// fixture scenario: Claude and Codex sessions whose background tasks the chat
// read shows. The spec package wires these shapes into sessions.
package bgtx

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	sec    = time.Second
	minute = time.Minute
)

// Paths are the folders a seed writes into: shell output files and subagent
// symlinks, subagent files, and the Codex home's sessions folder.
type Paths struct{ Tasks, Subagents, Rollouts string }

// File is one file the seeder writes. A Link makes Path a symlink to Link.
// Key records Path in Output.TaskFiles under that name.
type File struct {
	Path, Content, Link, Key string
	Mtime                    time.Time
}

// Seed is what a session gets after the fake wrote its own transcript: Lines
// are appended to it and Files are written beside it.
type Seed struct {
	Lines   []string
	Files   []File
	Version string        // written as the Codex rollout's cli_version when set
	Age     time.Duration // how long before the seed the session was created; tasks older than the run need a longer one
	Working bool          // also wait for the chat read's Working row
	// Ready, when set, replaces the task-count wait: the call descriptions the
	// parent's tools entries must name. Calls are the call ids by name, printed in
	// Output.Subagents.
	Ready []string
	Calls map[string]string
}

type entry struct {
	at   time.Time
	line string
}

// Lines builds one session's transcript lines and files. All times are
// offsets before the seed time, truncated to the second.
type Lines struct {
	now     time.Time
	p       Paths
	entries []entry
	files   []File
}

func NewLines(now time.Time, p Paths) *Lines {
	return &Lines{now: now.Truncate(time.Second), p: p}
}

// Seed is the lines in time order and the files.
func (b *Lines) Seed() Seed {
	slices.SortStableFunc(b.entries, func(x, y entry) int { return x.at.Compare(y.at) })
	out := Seed{Files: b.files}
	for _, e := range b.entries {
		out.Lines = append(out.Lines, e.line)
	}
	return out
}

func (b *Lines) when(ago time.Duration) time.Time { return b.now.Add(-ago) }
func (b *Lines) outputPath(taskID string) string  { return b.p.Tasks + "/" + taskID + ".output" }
func (b *Lines) file(f File)                      { b.files = append(b.files, f) }

func jsonLine(m map[string]any) string {
	raw, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func (b *Lines) add(ago time.Duration, m map[string]any) {
	at := b.when(ago)
	m["timestamp"] = at.UTC().Format(time.RFC3339Nano)
	m["uuid"] = fmt.Sprintf("bg-%03d", len(b.entries))
	b.entries = append(b.entries, entry{at, jsonLine(m)})
}

func toolUse(id, name string, input map[string]any) any {
	return map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}
}

// use is an assistant line with one tool call.
func (b *Lines) use(ago time.Duration, id, name string, input map[string]any) {
	b.add(ago, map[string]any{"type": "assistant", "message": map[string]any{"id": "msg_" + id, "role": "assistant",
		"content": []any{toolUse(id, name, input)}}})
}

// toolResult is the user line answering a tool call; res is its toolUseResult.
func toolResult(id, text string, res map[string]any) map[string]any {
	m := map[string]any{"type": "user", "message": map[string]any{"role": "user",
		"content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "content": text}}}}
	if res != nil {
		m["toolUseResult"] = res
	}
	return m
}

func (b *Lines) result(ago time.Duration, id, text string, res map[string]any) {
	b.add(ago, toolResult(id, text, res))
}

func (b *Lines) notify(ago time.Duration, body string) {
	b.add(ago, map[string]any{"type": "queue-operation", "operation": "enqueue", "content": "<task-notification>\n" + body + "\n</task-notification>"})
}

// monitor starts a Monitor at ago; its result lands a second later.
func (b *Lines) monitor(ago time.Duration, useID, taskID string, timeoutMs int64) {
	b.use(ago, useID, "Monitor", map[string]any{"command": "tail -f logs/app.log", "timeout_ms": timeoutMs, "description": "Watch the app log"})
	b.result(ago-time.Second, useID, "Monitor started: "+taskID, map[string]any{"taskId": taskID, "timeoutMs": timeoutMs})
}

// outputText is n lines of about width bytes each.
func outputText(label string, n, width int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		line := fmt.Sprintf("%s line %02d ", label, i)
		fmt.Fprintf(&sb, "%s%s\n", line, strings.Repeat(".", max(width-len(line)-1, 0)))
	}
	return sb.String()
}

// shell launches a background shell at ago and writes its output file. The
// launch result lands a second later and names the file.
func (b *Lines) shell(ago time.Duration, useID, taskID, command, output string, mtime time.Duration) {
	b.use(ago, useID, "Bash", map[string]any{"command": command, "run_in_background": true})
	path := b.outputPath(taskID)
	b.result(ago-time.Second, useID, "Command running in background with ID: "+taskID+". Output is being written to: "+path,
		map[string]any{"backgroundTaskId": taskID})
	b.file(File{Path: path, Content: output, Mtime: b.when(mtime)})
}

// endShell is the final notification of a shell.
func (b *Lines) endShell(ago time.Duration, taskID, status, summary string) {
	b.notify(ago, fmt.Sprintf("<task-id>%s</task-id>\n<output-file>%s</output-file>\n<status>%s</status>\n<summary>%s</summary>",
		taskID, b.outputPath(taskID), status, summary))
}

// timedOut is a Bash call that ran past its timeout and moved to the background.
func (b *Lines) timedOut(ago, doneAgo time.Duration, useID, taskID, command, output string) {
	b.use(ago, useID, "Bash", map[string]any{"command": command, "timeout": 120000})
	b.result(doneAgo, useID, "Command exceeded the timeout and moved to the background. Output is being written to: "+b.outputPath(taskID),
		map[string]any{"backgroundTaskId": taskID, "timedOutAfterMs": 120000})
	b.file(File{Path: b.outputPath(taskID), Content: output, Mtime: b.when(30 * time.Second)})
}

const (
	agentID = "a5c0de01" // the subagent most shapes launch first
	model   = "claude-sonnet-5-5"
)

// subagent launches an async subagent at ago; its file holds fileLines and its
// symlink sits in the tasks folder, as Claude Code writes them.
func (b *Lines) subagent(ago time.Duration, id, desc string, fileLines []string) {
	useID := "toolu_agent_" + id
	b.use(ago, useID, "Agent", map[string]any{"description": desc, "subagent_type": "Explore", "prompt": desc + " and report back."})
	file, link := b.p.Subagents+"/agent-"+id+".jsonl", b.outputPath(id)
	b.result(ago-time.Second, useID, "Async agent launched successfully.\nagentId: "+id,
		map[string]any{"status": "async_launched", "agentId": id, "resolvedModel": model, "outputFile": link})
	b.file(File{Path: file, Content: strings.Join(fileLines, "\n") + "\n", Mtime: b.when(ago - time.Second)})
	b.file(File{Path: link, Link: file})
}

// The lines of a subagent's own file are all sidechain lines.
func subLine(m map[string]any) string {
	m["isSidechain"] = true
	return jsonLine(m)
}

func subAssistant(msgID string, in, out int, blocks ...any) string {
	return subLine(map[string]any{"type": "assistant", "message": map[string]any{"id": msgID, "model": model, "role": "assistant",
		"content": blocks, "usage": map[string]any{"input_tokens": in, "output_tokens": out}}})
}

func subResult(id, text string, res map[string]any) string { return subLine(toolResult(id, text, res)) }

// The tokens and tool calls of the survey subagent's file.
const (
	SurveyTokens    = 4720
	SurveyToolCalls = 3
	SurveyCurrent   = "InFlightTool"
)

// surveyFile has usage on three message ids (one repeated by two parallel
// calls), three tool calls and an open Grep.
func surveyFile() []string {
	return []string{
		subAssistant("msg_s1", 1200, 40, map[string]any{"type": "text", "text": "Listing the sessions code."}),
		subAssistant("msg_s2", 1500, 60, toolUse("toolu_s1", "Read", map[string]any{"file_path": "server/sessions/sessions.go"})),
		subResult("toolu_s1", "package sessions", nil),
		subAssistant("msg_s3", 1800, 120, toolUse("toolu_s2", "Glob", map[string]any{"pattern": "server/sessions/*.go"})),
		subAssistant("msg_s3", 1800, 120, toolUse("toolu_s3", "Grep", map[string]any{"pattern": SurveyCurrent})),
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
