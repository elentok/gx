package bgtx

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"
)

// Names the tests and flows of the subagents scenario look at. The Call IDs are
// the parent's Agent tool_use ids, the Agent IDs the subagents' file names.
const (
	SubRunningCall  = "toolu_sub_running"
	SubFinishedCall = "toolu_sub_finished"
	SubMissingCall  = "toolu_sub_missing"

	SubRunningAgent  = "a5ub0001"
	SubFinishedAgent = "a5ub0002"

	SubRunningDesc  = "Survey the sessions code"
	SubFinishedDesc = "Check the wire types"
	SubMissingDesc  = "Read the old notes"

	SubHandback = "The wire types match: SessionTask has the model and token fields."

	SubStoppedCall    = "toolu_sub_stopped"
	SubBackgroundCall = "toolu_sub_background"
	SubNestedCall     = "toolu_sub_nested"
	SubGrandchildCall = "toolu_sub_grandchild"
	SubStartingCall   = "toolu_sub_starting"
	SubMarkedCall     = "toolu_sub_marked"

	SubStoppedAgent    = "a5ub0004"
	SubBackgroundAgent = "a5ub0005"
	SubNestedAgent     = "a5ub0006"
	SubGrandchildAgent = "a5ub0007"
	SubStartingAgent   = "a5ub0008"
	SubMarkedAgent     = "a5ub0009"

	SubStoppedDesc    = "Trace the stop path"
	SubBackgroundDesc = "Watch the build in the background"
	SubNestedDesc     = "Map the server packages"
	SubGrandchildDesc = "Read the wire package"
	SubStartingDesc   = "Warm up the new worker"
	SubMarkedDesc     = "Summarise the config files"

	// The string-background shape's subagent and the shell launched beside it.
	SubStringCall      = "toolu_sub_string"
	SubStringShellCall = "toolu_sub_string_shell"
	SubStringAgent     = "a5ub0010"
	SubStringDesc      = "Audit the string flags"
	SubStringShellDesc = "Lint in the background"
	SubStringFileKey   = "subagent-string-file"
	SubStringMetaKey   = "subagent-string-meta"

	// SubMarkedLast is the last assistant message of the marked-finished subagent,
	// which has no handback.
	SubMarkedLast = "The config files only set paths."

	// Keys of Seed.Files in Output.TaskFiles.
	SubRunningFileKey    = "subagent-running-file"
	SubRunningMetaKey    = "subagent-running-meta"
	SubFinishedFileKey   = "subagent-finished-file"
	SubFinishedMetaKey   = "subagent-finished-meta"
	SubStoppedFileKey    = "subagent-stopped-file"
	SubStoppedMetaKey    = "subagent-stopped-meta"
	SubBackgroundFileKey = "subagent-background-file"
	SubBackgroundMetaKey = "subagent-background-meta"
	SubNestedFileKey     = "subagent-nested-file"
	SubNestedMetaKey     = "subagent-nested-meta"
	SubGrandchildFileKey = "subagent-grandchild-file"
	SubGrandchildMetaKey = "subagent-grandchild-meta"
	SubStartingFileKey   = "subagent-starting-file" // not on disk at start: a flow writes it
	SubStartingMetaKey   = "subagent-starting-meta" // not on disk at start: a flow writes it
	SubMarkedFileKey     = "subagent-marked-file"
	SubMarkedMetaKey     = "subagent-marked-meta"
)

// Subagent is one subagents-scenario shape: its lines, the call ids it
// launched by name (Output.Subagents.Calls) and the call descriptions the chat
// read must list before the session is ready.
type Subagent struct {
	Codex bool // the parent is a Codex session
	Build func(b *Lines)
	Calls map[string]string
	Ready []string
}

var (
	runningCalls  = map[string]string{"running": SubRunningCall}
	finishedCalls = map[string]string{"finished": SubFinishedCall}
	missingCalls  = map[string]string{"missing": SubMissingCall}
)

// SubagentShapes maps a --shape of the subagents scenario to its build.
var SubagentShapes = map[string]Subagent{
	"claude": {
		Build: func(b *Lines) {
			b.subFinished(5 * minute)
			b.subMissing(4 * minute)
			b.subRunning(3 * minute)
		},
		Calls: map[string]string{"running": SubRunningCall, "finished": SubFinishedCall, "missing": SubMissingCall},
		Ready: []string{SubRunningDesc, SubFinishedDesc, SubMissingDesc},
	},
	"foreground-running": {Build: func(b *Lines) { b.subRunning(3 * minute) }, Calls: runningCalls, Ready: []string{SubRunningDesc}},
	"finished-handback":  {Build: func(b *Lines) { b.subFinished(3 * minute) }, Calls: finishedCalls, Ready: []string{SubFinishedDesc}},
	"file-gone":          {Build: func(b *Lines) { b.subMissing(3 * minute) }, Calls: missingCalls, Ready: []string{SubMissingDesc}},

	"stopped-no-handback":  {Build: func(b *Lines) { b.subStopped(10 * minute) }, Calls: map[string]string{"stopped": SubStoppedCall}, Ready: []string{SubStoppedDesc}},
	"background-returned":  {Build: func(b *Lines) { b.subBackground(30 * minute) }, Calls: map[string]string{"background": SubBackgroundCall}, Ready: []string{SubBackgroundDesc}},
	"nested":               {Build: func(b *Lines) { b.subNested(3 * minute) }, Calls: map[string]string{"nested": SubNestedCall, "grandchild": SubGrandchildCall}, Ready: []string{SubNestedDesc}},
	"starting":             {Build: func(b *Lines) { b.subStarting(20 * sec) }, Calls: map[string]string{"starting": SubStartingCall}, Ready: []string{SubStartingDesc}},
	"finished-no-handback": {Build: func(b *Lines) { b.subMarked(5 * minute) }, Calls: map[string]string{"marked": SubMarkedCall}, Ready: []string{SubMarkedDesc}},
	"string-background":    {Build: func(b *Lines) { b.subStringBackground(3 * minute) }, Calls: map[string]string{"string": SubStringCall}, Ready: []string{SubStringDesc}},
	"parent-gone":          {Build: func(b *Lines) { b.subRunning(3 * minute) }, Calls: runningCalls, Ready: []string{SubRunningDesc}},
	"exited":               {Build: func(b *Lines) { b.subRunning(3 * minute) }, Calls: runningCalls, Ready: []string{SubRunningDesc}},
}

// CallsShown reports whether the chat read has a tools entry naming every Ready call.
func (s Seed) CallsShown(c wire.SessionChatResponse) bool {
	return slices.ContainsFunc(c.Messages, func(m wire.SessionChatMessage) bool {
		return m.Kind == "tools" && !slices.ContainsFunc(s.Ready, func(want string) bool {
			return !slices.ContainsFunc(m.Calls, func(call string) bool { return strings.Contains(call, want) })
		})
	})
}

// meta is a subagent's meta.json; Finished is absent while it runs.
type meta struct {
	AgentType    string `json:"agentType"`
	Description  string `json:"description"`
	ToolUseID    string `json:"toolUseId"`
	SpawnDepth   int    `json:"spawnDepth"`
	RequestShape string `json:"requestShape"`
	Model        string `json:"model"`
	Finished     bool   `json:"finished,omitempty"`
	ParentAgent  string `json:"parentAgentId,omitempty"`
}

// subagentCall is the parent's Agent call plus the subagent's file and
// meta.json. The call has no result; the caller adds one.
func (b *Lines) subagentCall(ago time.Duration, useID, id, desc string, finished bool, fileLines []string, fileKey, metaKey string) {
	b.subCall(ago, useID, desc)
	b.subFiles(ago, id, meta{Description: desc, ToolUseID: useID, SpawnDepth: 1, RequestShape: "foreground", Finished: finished}, fileLines, fileKey, metaKey)
}

func (b *Lines) subCall(ago time.Duration, useID, desc string) {
	b.use(ago, useID, "Agent", map[string]any{"description": desc, "subagent_type": "Explore", "prompt": desc + " and report back."})
}

// metaFor fills the fields every meta.json has.
func metaFor(m meta) []byte {
	m.AgentType, m.Model = "Explore", model
	raw, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return append(raw, '\n')
}

// subFiles is a subagent's file, which starts with its prompt, and its meta.json.
func (b *Lines) subFiles(ago time.Duration, id string, m meta, fileLines []string, fileKey, metaKey string) {
	prompt := subLine(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": m.Description + " and report back."}})
	base := b.p.Subagents + "/agent-" + id
	b.file(File{Path: base + ".jsonl", Content: strings.Join(append([]string{prompt}, fileLines...), "\n") + "\n", Mtime: b.when(ago - 2*time.Second), Key: fileKey})
	b.file(File{Path: base + ".meta.json", Content: string(metaFor(m)), Mtime: b.when(ago - time.Second), Key: metaKey})
}

// subStopped is a foreground call that returned with the subagent's last
// message, without a handback in its file or a finished marker.
func (b *Lines) subStopped(ago time.Duration) {
	file := []string{
		subAssistant("msg_x1", 900, 50, toolUse("toolu_x1", "Grep", map[string]any{"pattern": "Stop"})),
		subResult("toolu_x1", "server/sessions/stop.go", nil),
		subAssistant("msg_x2", 1300, 80, map[string]any{"type": "text", "text": "Stopped before reporting."}),
	}
	b.subagentCall(ago, SubStoppedCall, SubStoppedAgent, SubStoppedDesc, false, file, SubStoppedFileKey, SubStoppedMetaKey)
	b.result(ago-time.Minute, SubStoppedCall, "The subagent was stopped.", map[string]any{"status": "completed", "agentId": SubStoppedAgent, "resolvedModel": model})
}

// subBackground is a background launch whose call returned async_launched long
// ago, with the symlink in the tasks folder; nothing ended it.
func (b *Lines) subBackground(ago time.Duration) {
	b.subCall(ago, SubBackgroundCall, SubBackgroundDesc)
	b.subLaunched(ago, SubBackgroundCall, SubBackgroundAgent, SubBackgroundDesc, SubBackgroundFileKey, SubBackgroundMetaKey)
}

// subLaunched is everything a background Agent call started at ago leaves
// behind: its async_launched result, file, meta.json and tasks-folder symlink.
func (b *Lines) subLaunched(ago time.Duration, useID, id, desc, fileKey, metaKey string) {
	file, link := b.p.Subagents+"/agent-"+id+".jsonl", b.outputPath(id)
	b.result(ago-time.Second, useID, "Async agent launched successfully.\nagentId: "+id,
		map[string]any{"status": "async_launched", "agentId": id, "description": desc, "resolvedModel": model, "outputFile": link})
	b.subFiles(ago, id, meta{Description: desc, ToolUseID: useID, SpawnDepth: 1, RequestShape: "background"}, surveyFile(), fileKey, metaKey)
	b.file(File{Path: link, Link: file})
}

// subStringBackground is one assistant line that launches a background subagent
// and a background shell, both with run_in_background as a string, as Claude
// Code often writes it.
func (b *Lines) subStringBackground(ago time.Duration) {
	b.add(ago, map[string]any{"type": "assistant", "message": map[string]any{"id": "msg_" + SubStringCall, "role": "assistant", "content": []any{
		toolUse(SubStringCall, "Agent", map[string]any{"description": SubStringDesc, "subagent_type": "Explore", "prompt": SubStringDesc + " and report back.", "run_in_background": "true"}),
		toolUse(SubStringShellCall, "Bash", map[string]any{"description": SubStringShellDesc, "command": "make lint", "run_in_background": "true"}),
	}}})
	b.subLaunched(ago, SubStringCall, SubStringAgent, SubStringDesc, SubStringFileKey, SubStringMetaKey)
	b.shellLaunched(ago, SubStringShellCall, "b5ub0001", outputText("lint", 3, 40), ago-time.Second)
}

// subNested is a subagent whose file has an open Agent call; the grandchild's
// file sits flat beside it, its meta.json pointing into the parent subagent's file.
func (b *Lines) subNested(ago time.Duration) {
	// The task scan drops a line with no timestamp, and the nested call must be found in this file.
	child := []string{subLine(map[string]any{"type": "assistant", "timestamp": b.when(ago).UTC().Format(time.RFC3339Nano),
		"message": map[string]any{"id": "msg_n1", "model": model, "role": "assistant",
			"content": []any{toolUse(SubGrandchildCall, "Agent",
				map[string]any{"description": SubGrandchildDesc, "subagent_type": "Explore", "prompt": SubGrandchildDesc + " and report back."})},
			"usage": map[string]any{"input_tokens": 1000, "output_tokens": 70}}})}
	b.subagentCall(ago, SubNestedCall, SubNestedAgent, SubNestedDesc, false, child, SubNestedFileKey, SubNestedMetaKey)
	b.subFiles(ago-sec, SubGrandchildAgent, meta{Description: SubGrandchildDesc, ToolUseID: SubGrandchildCall, SpawnDepth: 2, RequestShape: "foreground", ParentAgent: SubNestedAgent},
		surveyFile(), SubGrandchildFileKey, SubGrandchildMetaKey)
}

// subStarting is a call that runs while its subagent has no file yet: the
// paths a flow writes are reserved, not created.
func (b *Lines) subStarting(ago time.Duration) {
	b.subCall(ago, SubStartingCall, SubStartingDesc)
	base := b.p.Subagents + "/agent-" + SubStartingAgent
	b.reserve(File{Path: base + ".jsonl", Key: SubStartingFileKey}, File{Path: base + ".meta.json", Key: SubStartingMetaKey})
}

// subMarked is a subagent whose meta.json says finished while its file has no
// handback; the call returned.
func (b *Lines) subMarked(ago time.Duration) {
	file := []string{
		subAssistant("msg_m1", 700, 40, toolUse("toolu_m1", "Glob", map[string]any{"pattern": "**/*.toml"})),
		subResult("toolu_m1", "config.toml", nil),
		subAssistant("msg_m2", 1100, 60, map[string]any{"type": "text", "text": SubMarkedLast}),
	}
	b.subagentCall(ago, SubMarkedCall, SubMarkedAgent, SubMarkedDesc, true, file, SubMarkedFileKey, SubMarkedMetaKey)
	b.result(ago-time.Minute, SubMarkedCall, SubMarkedLast, map[string]any{"status": "completed", "agentId": SubMarkedAgent, "resolvedModel": model})
}

// subRunning is a foreground subagent: no result on the parent call, no
// finished in the meta, and a file a flow can grow.
func (b *Lines) subRunning(ago time.Duration) {
	b.subagentCall(ago, SubRunningCall, SubRunningAgent, SubRunningDesc, false, surveyFile(), SubRunningFileKey, SubRunningMetaKey)
}

// subFinished is a subagent whose file ends with its handback and whose parent
// call has the result.
func (b *Lines) subFinished(ago time.Duration) {
	file := []string{
		subAssistant("msg_f1", 800, 60, toolUse("toolu_f1", "Read", map[string]any{"file_path": "server/wire/sessions.go"})),
		subResult("toolu_f1", "package wire", nil),
		subAssistant("msg_f2", 1400, 90, toolUse("toolu_f2", "SubagentHandback", map[string]any{"message": SubHandback})),
		subResult("toolu_f2", "Handback delivered", nil),
	}
	b.subagentCall(ago, SubFinishedCall, SubFinishedAgent, SubFinishedDesc, true, file, SubFinishedFileKey, SubFinishedMetaKey)
	b.result(ago-time.Minute, SubFinishedCall, SubHandback, map[string]any{"status": "completed", "agentId": SubFinishedAgent, "resolvedModel": model})
}

// subMissing is a finished call whose subagent file is not on disk.
func (b *Lines) subMissing(ago time.Duration) {
	b.use(ago, SubMissingCall, "Agent", map[string]any{"description": SubMissingDesc, "subagent_type": "Explore", "prompt": SubMissingDesc + " and report back."})
	b.result(ago-time.Minute, SubMissingCall, "Nothing relevant found.", map[string]any{"status": "completed", "agentId": "a5ub0003", "resolvedModel": model})
}
