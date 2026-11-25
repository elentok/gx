package chat

import (
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/chatreader"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/subagent"
	"github.com/elentok/gx/transcript"
)

const noSuchSubagent = "no such subagent"

// subagentRead answers a chat read with subagent=<call id>: the page of one
// Claude subagent. Codex children are not served yet.
func (c *Chat) subagentRead(w http.ResponseWriter, id, dir string, meta Meta, callID string) {
	path, kind, found := c.Transcript(id, meta)
	if !found {
		c.writeParentGone(w, id, meta, callID)
		return
	}
	parent, err := c.ReadSnapshot(path, kind, meta.Flow, dir)
	if os.IsNotExist(err) {
		c.writeParentGone(w, id, meta, callID)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if kind == harness.Codex {
		c.codexSubagentRead(w, id, dir, meta, parent.Records, callID)
		return
	}
	if kind != harness.Claude {
		http.Error(w, noSuchSubagent, http.StatusNotFound)
		return
	}
	rec, inParent := recordOfCall(parent.Records, callID)
	if !inParent {
		rec = transcript.TaskRecord{Kind: transcript.TaskKindSubagent, CallID: callID}
	}
	ref, ok := c.claudeSubagent(rec, path)
	var above []subagent.SessionSubagentLink
	if ok && !inParent {
		// A call the parent does not hold is a nested one: its record is in
		// the file of the subagent that made it.
		rec, above, inParent = c.nestedCall(parent.Records, path, rec, ref)
	}
	var snap Snapshot
	if ok {
		snap, err = c.subagentPages.Read(ref.Path, kind, false, c.subagentLoader(dir))
	}
	if !ok || os.IsNotExist(err) {
		if !inParent {
			http.Error(w, noSuchSubagent, http.StatusNotFound)
			return
		}
		wire.WriteJSON(w, c.missingSubagentPage(id, meta, rec))
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rec = c.withSubagentDetails([]transcript.TaskRecord{rec}, path)[0]

	// resolveTask is the one rule set of the task row. A call no file holds
	// has no record to resolve.
	var state taskState
	if inParent {
		state = resolveTask(rec, meta, c.clock())
	}
	head := &subagent.SessionSubagent{
		CallID: callID, RootSessionID: id, Title: rec.Label, Type: ref.AgentType,
		Chain: above,
		Model: rec.Model, Tokens: rec.Tokens, ToolCalls: rec.ToolCalls,
		Result: snap.Handback,
	}
	if n := len(above); n > 0 {
		head.ParentCallID = above[n-1].CallID
	}
	switch {
	case snap.Handback != "":
		head.Status = subagent.SubagentStatusDone
	case state.finished || ref.Finished:
		head.Status = subagent.SubagentStatusStopped
		if ref.Finished {
			head.Result = lastAgentText(snap.Msgs)
		}
	default:
		head.Status = subagent.SubagentStatusRunning
	}
	if !rec.StartedAt.IsZero() {
		head.StartedAt = rec.StartedAt.Format(time.RFC3339)
	}
	if state.finished && !state.noDuration {
		head.EndedAt = state.at.Format(time.RFC3339)
	}

	out := wire.SessionChatResponse{
		ConversationID: meta.ConversationID, Questions: []wire.QuestionCard{},
		Messages: snap.Msgs, Truncated: snap.Truncated, Subagent: head,
	}
	if len(out.Messages) > 0 && out.Messages[0].Role == transcript.ChatRolePerson {
		head.Prompt, out.Messages = out.Messages[0].Text, out.Messages[1:]
	}
	if head.Status == subagent.SubagentStatusRunning {
		out.Current, out.Others = currentCalls(snap.InFlight)
	}
	wire.WriteJSON(w, out)
}

// writeParentGone answers a known session whose parent transcript cannot be
// read. The parent's call cannot be read either, so the header holds no title
// or prompt, and an unknown call id looks the same as a known one.
func (c *Chat) writeParentGone(w http.ResponseWriter, id string, meta Meta, callID string) {
	head := &subagent.SessionSubagent{
		CallID: callID, RootSessionID: id,
		Status: subagent.SubagentStatusNotAvailable, Unavailable: subagent.SubagentUnavailableGone,
	}
	// Archived tells a compressed parent from a compressed child, which share
	// the reason.
	_, archived := c.codexArchive(id, meta)
	if archived {
		head.Unavailable = subagent.SubagentUnavailableCompressed
	}
	wire.WriteJSON(w, wire.SessionChatResponse{
		ConversationID: meta.ConversationID, Questions: []wire.QuestionCard{},
		Messages: []wire.SessionChatMessage{}, Subagent: head, Archived: archived,
	})
}

// nestedCall finds the record of a nested call in the file that holds it and
// the calls above it, outermost first. Each level's sidecar names the call that
// started it and the agent that made that call, up to the parent transcript.
// A level whose sidecar or record is missing ends the chain there.
func (c *Chat) nestedCall(parentRecords []transcript.TaskRecord, parentPath string, stub transcript.TaskRecord, ref subagentRef) (transcript.TaskRecord, []subagent.SessionSubagentLink, bool) {
	dir := strings.TrimSuffix(parentPath, ".jsonl") + "/subagents"
	holder := func(maker string) []transcript.TaskRecord {
		if maker == "" {
			return parentRecords
		}
		if !validAgentID(maker) {
			return nil
		}
		records, _ := c.subagentTasks.Read(dir+"/agent-"+maker+".jsonl", func(p string) ([]transcript.TaskRecord, error) {
			records, _, err := transcript.ReadSubagentTaskRecords(p)
			return records, err
		})
		return records
	}
	rec, found := recordOfCall(holder(ref.ParentAgentID), stub.CallID)
	if !found {
		return stub, nil, false
	}
	var above []subagent.SessionSubagentLink
	seen := map[string]bool{}
	for maker := ref.ParentAgentID; maker != "" && !seen[maker]; {
		seen[maker] = true
		m, err := c.subagentMetas.Read(dir+"/agent-"+maker+".meta.json", readSubagentMeta)
		if err != nil || m.ToolUseID == "" {
			break
		}
		up, ok := recordOfCall(holder(m.ParentAgentID), m.ToolUseID)
		if !ok {
			break
		}
		above = append(above, subagent.SessionSubagentLink{CallID: up.CallID, Title: up.Label})
		maker = m.ParentAgentID
	}
	slices.Reverse(above)
	return rec, above, true
}

// missingSubagentPage answers a call the parent holds but whose file is
// missing: the header comes from the parent's call alone. resolveTask decides
// whether the call still counts as running, so the file is only late.
func (c *Chat) missingSubagentPage(id string, meta Meta, rec transcript.TaskRecord) wire.SessionChatResponse {
	head := &subagent.SessionSubagent{
		CallID: rec.CallID, RootSessionID: id, Title: rec.Label, Type: rec.SubagentType,
		Prompt: rec.Prompt, Status: subagent.SubagentStatusRunning,
		Unavailable: subagent.SubagentUnavailableStarting,
	}
	if resolveTask(rec, meta, c.clock()).finished {
		head.Status, head.Unavailable = subagent.SubagentStatusNotAvailable, subagent.SubagentUnavailableGone
	}
	return wire.SessionChatResponse{
		ConversationID: meta.ConversationID, Questions: []wire.QuestionCard{},
		Messages: []wire.SessionChatMessage{}, Subagent: head,
	}
}

func recordOfCall(records []transcript.TaskRecord, callID string) (transcript.TaskRecord, bool) {
	for _, r := range records {
		if r.Kind == transcript.TaskKindSubagent && r.CallID == callID {
			return r, true
		}
	}
	return transcript.TaskRecord{}, false
}

func lastAgentText(msgs []wire.SessionChatMessage) string {
	for _, m := range slices.Backward(msgs) {
		if m.Role == transcript.ChatRoleAgent && m.Kind == transcript.ChatKindText {
			return m.Text
		}
	}
	return ""
}

// subagentLoader reads a subagent file for its page: the chat, the handback
// and the calls still open. sessionDir holds the images folder.
func (c *Chat) subagentLoader(sessionDir string) func(string) (Snapshot, error) {
	return func(path string) (Snapshot, error) {
		if c.reads.subagentPage != nil {
			c.reads.subagentPage(path)
		}
		f, err := os.Open(path)
		if err != nil {
			return Snapshot{}, err
		}
		defer f.Close()
		msgs, handback, err := transcript.ReadSubagentChatFrom(f, sessionDir+"/"+ImagesDir)
		if err != nil {
			return Snapshot{}, err
		}
		snap := SnapshotOf(chatreader.Result{Messages: msgs}, harness.Claude, FlowRef{}, false)
		snap.Handback = handback
		if snap.InFlight, err = transcript.ReadSubagentInFlightTools(path); err != nil {
			return Snapshot{}, err
		}
		return snap, nil
	}
}

// currentCalls splits the open calls into the newest and the older ones.
func currentCalls(open []transcript.InFlightTool) (current *wire.SessionInFlightTool, others []wire.SessionInFlightTool) {
	for _, t := range open {
		tool := wire.SessionInFlightTool{Kind: t.Kind, Label: t.Label}
		if current == nil {
			current = &tool
		} else {
			others = append(others, tool)
		}
	}
	return current, others
}
