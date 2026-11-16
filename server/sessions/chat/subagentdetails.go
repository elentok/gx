package chat

import (
	"path/filepath"
	"strings"

	"github.com/elentok/gx/transcript"
)

// subagentFilePath is where a subagent's own transcript is: the exact output
// path its launch or notification named, else the file next to the parent
// transcript. Both come from transcript text, so unsafe shapes give "".
func subagentFilePath(r transcript.TaskRecord, parentPath string) string {
	if safeShellOutputPath(r.OutputPath) {
		return r.OutputPath
	}
	if r.ID == "" || filepath.Base(r.ID) != r.ID || r.ID == "." || r.ID == ".." {
		return ""
	}
	return strings.TrimSuffix(parentPath, ".jsonl") + "/subagents/agent-" + r.ID + ".jsonl"
}

// withSubagentDetails returns records with each Claude subagent's model,
// tokens, tool calls and current call filled. It runs on every chat read and
// not in the snapshot: the subagent's file grows while the parent's is quiet.
// It copies what it changes: the records belong to the cached snapshot.
func (c *Chat) withSubagentDetails(records []transcript.TaskRecord, parentPath string) []transcript.TaskRecord {
	compute := transcript.ReadSubagentFile
	if c.readSubagentFile != nil {
		compute = c.readSubagentFile
	}
	var out []transcript.TaskRecord
	for i, r := range records {
		if r.Kind != transcript.TaskKindSubagent || r.ID == "" || r.UsageFinal {
			continue
		}
		path := subagentFilePath(r, parentPath)
		if path == "" {
			continue
		}
		d, err := c.subagentFiles.Read(path, compute)
		if err != nil || d == (transcript.SubagentDetails{}) {
			continue
		}
		if out == nil {
			out = make([]transcript.TaskRecord, len(records))
			copy(out, records)
		}
		r.Tokens, r.ToolCalls = d.Tokens, d.ToolCalls
		if d.Model != "" {
			r.Model = d.Model
		}
		if r.End == nil {
			r.CurrentCall = d.CurrentCall
		}
		out[i] = r
	}
	if out == nil {
		return records
	}
	return out
}
