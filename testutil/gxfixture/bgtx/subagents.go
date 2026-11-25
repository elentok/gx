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

	// Keys of Seed.Files in Output.TaskFiles.
	SubRunningFileKey  = "subagent-running-file"
	SubRunningMetaKey  = "subagent-running-meta"
	SubFinishedFileKey = "subagent-finished-file"
	SubFinishedMetaKey = "subagent-finished-meta"
)

// Subagent is one subagents-scenario shape: its lines, the call ids it
// launched by name (Output.Subagents.Calls) and the call descriptions the chat
// read must list before the session is ready.
type Subagent struct {
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
	b.use(ago, useID, "Agent", map[string]any{"description": desc, "subagent_type": "Explore", "prompt": desc + " and report back."})
	raw, err := json.Marshal(meta{AgentType: "Explore", Description: desc, ToolUseID: useID, RequestShape: "foreground", Model: model, Finished: finished})
	if err != nil {
		panic(err)
	}
	prompt := subLine(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": desc + " and report back."}})
	base := b.p.Subagents + "/agent-" + id
	b.file(File{Path: base + ".jsonl", Content: strings.Join(append([]string{prompt}, fileLines...), "\n") + "\n", Mtime: b.when(ago - 2*time.Second), Key: fileKey})
	b.file(File{Path: base + ".meta.json", Content: string(raw) + "\n", Mtime: b.when(ago - time.Second), Key: metaKey})
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
