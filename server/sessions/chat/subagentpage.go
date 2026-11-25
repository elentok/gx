package chat

import (
	"net/http"
	"os"
	"slices"
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
	if !found || kind != harness.Claude {
		http.Error(w, noSuchSubagent, http.StatusNotFound)
		return
	}
	parent, err := c.ReadSnapshot(path, kind, meta.Flow, dir)
	if os.IsNotExist(err) {
		http.Error(w, noSuchSubagent, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rec, inParent := recordOfCall(parent.Records, callID)
	if !inParent {
		rec = transcript.TaskRecord{Kind: transcript.TaskKindSubagent, CallID: callID}
	}
	ref, ok := c.claudeSubagent(rec, path)
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

	// resolveTask is the one rule set of the task row. A call the parent does
	// not hold (a nested one) has no record to resolve.
	var state taskState
	if inParent {
		state = resolveTask(rec, meta, c.clock())
	}
	head := &subagent.SessionSubagent{
		CallID: callID, RootSessionID: id, Title: rec.Label, Type: ref.AgentType,
		Model: rec.Model, Tokens: rec.Tokens, ToolCalls: rec.ToolCalls,
		Result: snap.Handback,
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
