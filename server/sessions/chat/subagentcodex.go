package chat

import (
	"net/http"
	"os"
	"time"

	"github.com/elentok/gx/chatreader"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/subagent"
	"github.com/elentok/gx/transcript"
)

// codexPromptUnreadable stands in for a Codex child's prompt: it is an
// encrypted agent_message, so only a NEW_TASK header could be shown.
const codexPromptUnreadable = "Prompt not readable"

// codexSubagentRead answers a chat read with subagent=<call id> for a Codex
// session: the page of one child rollout. The call links to the child through
// the record's thread id, so a call the parent does not hold is unknown: the
// server never looks inside a child for more calls.
func (c *Chat) codexSubagentRead(w http.ResponseWriter, id, dir string, meta Meta, records []transcript.TaskRecord, callID string) {
	rec, ok := recordOfCall(records, callID)
	if !ok {
		http.Error(w, noSuchSubagent, http.StatusNotFound)
		return
	}
	// Like the parent's read, this runs on an event: a remembered miss from
	// before the rollout existed would hide it.
	c.transcripts.ForgetMiss(rec.ID)
	rec = c.withCodexChildren([]transcript.TaskRecord{rec}, meta)[0]
	ref, found := c.codexSubagent(rec, meta)
	var snap Snapshot
	var err error
	if found {
		snap, err = c.subagentPages.Read(ref.Path, harness.Codex, false, c.codexChildLoader(dir))
	}
	if !found || os.IsNotExist(err) {
		wire.WriteJSON(w, c.missingCodexPage(id, meta, rec))
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	state := resolveTask(rec, meta, c.clock())
	head := &subagent.SessionSubagent{
		CallID: callID, RootSessionID: id, Title: rec.Label, Type: rec.SubagentType,
		Model: rec.Model, Tokens: rec.Tokens,
		Prompt: codexPromptUnreadable, Result: lastAgentText(snap.Msgs),
	}
	switch {
	case state.finished && rec.End != nil:
		head.Status = subagent.SubagentStatusDone
	case state.finished:
		head.Status = subagent.SubagentStatusStopped
	default:
		head.Status = subagent.SubagentStatusRunning
	}
	if !rec.StartedAt.IsZero() {
		head.StartedAt = rec.StartedAt.Format(time.RFC3339)
	}
	if state.finished && !state.noDuration {
		head.EndedAt = state.at.Format(time.RFC3339)
	}

	msgs := snap.Msgs
	if len(msgs) > 0 && msgs[0].Role == transcript.ChatRolePerson {
		msgs = msgs[1:]
	}
	out := wire.SessionChatResponse{
		ConversationID: meta.ConversationID, Questions: []wire.QuestionCard{},
		Messages: msgs, Truncated: snap.Truncated, Subagent: head,
	}
	if head.Status == subagent.SubagentStatusRunning {
		out.Current, out.Others = currentCalls(snap.InFlight)
	}
	wire.WriteJSON(w, out)
}

// missingCodexPage answers a call whose child rollout cannot be found. A
// rollout Codex compressed is not read yet.
func (c *Chat) missingCodexPage(id string, meta Meta, rec transcript.TaskRecord) wire.SessionChatResponse {
	out := c.missingSubagentPage(id, meta, rec)
	out.Subagent.Prompt = codexPromptUnreadable
	if rec.ID != "" {
		if home, err := c.src.CodexHome(); err == nil {
			if _, ok, _ := codexsession.ArchivedRolloutIn(home, rec.ID); ok {
				out.Subagent.Status = subagent.SubagentStatusNotAvailable
				out.Subagent.Unavailable = subagent.SubagentUnavailableCompressed
			}
		}
	}
	return out
}

// codexChildLoader reads a child rollout as the child's own chat, without the
// copy of the parent's history that opens the file.
func (c *Chat) codexChildLoader(sessionDir string) func(string) (Snapshot, error) {
	return func(path string) (Snapshot, error) {
		if c.reads.subagentPage != nil {
			c.reads.subagentPage(path)
		}
		meta, err := codexsession.ReadChildMeta(path)
		if err != nil {
			return Snapshot{}, err
		}
		f, err := os.Open(path)
		if err != nil {
			return Snapshot{}, err
		}
		defer f.Close()
		chat, err := codexsession.ReadChatFromOrdinal(f, sessionDir+"/"+ImagesDir, meta.HistoryStartOrdinal)
		if err != nil {
			return Snapshot{}, err
		}
		return SnapshotOf(chatreader.Result{Messages: chat.Messages, CodexVersion: chat.Version}, harness.Codex, FlowRef{}, false), nil
	}
}
