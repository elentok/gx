package chat

import "github.com/elentok/gx/transcript"

// withSubagentDetails returns records with each Claude subagent's model,
// tokens, tool calls and current call filled. It runs on every chat read and
// not in the snapshot: the subagent's file grows while the parent's is quiet.
// It copies what it changes: the records belong to the cached snapshot. workDir
// is the session's, for the current call's label.
func (c *Chat) withSubagentDetails(records []transcript.TaskRecord, parentPath, workDir string) []transcript.TaskRecord {
	compute := transcript.ReadSubagentFile
	if c.reads.subagent != nil {
		compute = c.reads.subagent
	}
	return enrichTasks(records, enrichSpec[transcript.SubagentDetails]{
		cache: &c.subagentFiles, compute: compute,
		path: func(r transcript.TaskRecord) (string, bool) {
			if r.Kind != transcript.TaskKindSubagent || r.UsageFinal {
				return "", false
			}
			ref, ok := c.claudeSubagent(r, parentPath)
			return ref.Path, ok
		},
		usable: func(d transcript.SubagentDetails) bool { return d != (transcript.SubagentDetails{}) },
		merge: func(r *transcript.TaskRecord, d transcript.SubagentDetails) {
			r.Tokens, r.ToolCalls = d.Tokens, d.ToolCalls
			if d.Model != "" {
				r.Model = d.Model
			}
			if r.End == nil {
				r.CurrentCall, r.CurrentCallPath = d.CurrentCall.LabelIn(workDir)
			}
		},
	})
}
