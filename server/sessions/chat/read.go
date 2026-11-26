package chat

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/chatreader"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

// readable resolves a read's session id, or answers the refusal or error and
// returns ok false.
func (c *Chat) readable(w http.ResponseWriter, id string) (dir string, meta Meta, ok bool) {
	dir, meta, res, err := c.src.ReadableSession(id)
	if res.Refused {
		status := http.StatusBadRequest
		if res.Reason == wire.ReasonSessionUnknown {
			status = http.StatusNotFound
		}
		http.Error(w, res.Message, status)
		return "", Meta{}, false
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return "", Meta{}, false
	}
	return dir, meta, true
}

// transcriptRead names the conversation file of a session, by the same lookup
// the chat uses, so the CLI and the web chat agree.
func (c *Chat) transcriptRead(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session")
	_, meta, ok := c.readable(w, id)
	if !ok {
		return
	}
	out := wire.SessionTranscriptResponse{Harness: meta.Harness, ConversationID: meta.ConversationID}
	if c.src.HasChatReader(meta.Harness) {
		out.Path, _, out.Found = c.Transcript(id, meta)
	}
	wire.WriteJSON(w, out)
}

// archiveRead decodes a session's archived rollout on demand and answers like
// a plain chat read would.
func (c *Chat) archiveRead(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session")
	dir, meta, ok := c.readable(w, id)
	if !ok {
		return
	}
	path, ok := c.codexArchive(id, meta)
	if !ok {
		http.Error(w, "no archived rollout for this session", http.StatusNotFound)
		return
	}
	read, err := chatreader.ReadZstdFile(harness.Codex, path, filepath.Join(dir, ImagesDir))
	if err != nil {
		http.Error(w, "cannot decode the archived rollout: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	snap := SnapshotOf(read, harness.Codex, meta.Flow, c.src.StepFinal(meta.Flow))
	wire.WriteJSON(w, wire.SessionChatResponse{
		// A decoded archive has no task records.
		ConversationID: meta.ConversationID, Messages: withCallStates(snap.Msgs, nil, meta, c.clock()), Truncated: snap.Truncated,
		Questions: SessionCards(meta, snap.Round), Next: snap.Next,
		Failure:    c.Failure(meta),
		Unreadable: snap.Unreadable, CodexVersion: snap.CodexVersion,
	})
}

func (c *Chat) read(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session")
	dir, meta, ok := c.readable(w, id)
	if !ok {
		return
	}
	out := wire.SessionChatResponse{ConversationID: meta.ConversationID, Messages: []wire.SessionChatMessage{}, Questions: c.Cards(dir, meta, Snapshot{}), Failure: c.Failure(meta)}
	if !c.src.HasChatReader(meta.Harness) {
		out.Unsupported = true
		wire.WriteJSON(w, out)
		return
	}
	if call := r.URL.Query().Get("subagent"); call != "" {
		c.subagentRead(w, id, dir, meta, call)
		return
	}
	// The page reads only on an event, so a remembered miss from before the
	// transcript existed would hide the message the event announces.
	c.transcripts.ForgetMiss(meta.ConversationID)
	path, kind, found := c.Transcript(id, meta)
	if !found {
		_, out.Archived = c.codexArchive(id, meta)
	} else {
		snap, err := c.ReadSnapshot(path, kind, meta.Flow, dir)
		switch {
		case os.IsNotExist(err): // vanished since the lookup: no chat yet
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		default:
			// One clock reading, so a call leaves the running rows and reads as
			// finished in the same response.
			now := c.clock()
			out.Messages, out.Truncated = withCallStates(snap.Msgs, snap.Records, meta, now), snap.Truncated
			out.Next = snap.Next
			var open []transcript.InFlightTool
			out.Tasks, open = c.tasksFor(snap, kind, meta, path, now)
			out.Questions = c.Cards(dir, meta, snap)
			out.Unreadable, out.CodexVersion = snap.Unreadable, snap.CodexVersion
			out.Current, out.Others = currentCalls(open, meta.Dir)
		}
	}
	wire.WriteJSON(w, out)
}

// tasksFor assembles the task list of a snapshot and returns the open calls
// that are not in it. Shell and Agent calls are tasks for harnesses that get
// the list.
func (c *Chat) tasksFor(snap Snapshot, kind agentrunner.Kind, meta Meta, path string, now time.Time) ([]wire.SessionTask, []transcript.InFlightTool) {
	records := snap.Records
	if kind == harness.Codex {
		records = c.withCodexChildren(records, meta)
	} else {
		records = c.withSubagentDetails(c.withShellOutputs(records), path, meta.Dir)
	}
	hasList := kind != harness.Codex || hasExecShape(snap.CodexVersion)
	var open []transcript.InFlightTool
	for _, t := range snap.InFlight {
		if hasList && (t.Kind == transcript.InFlightShell || t.Kind == transcript.InFlightAgent) {
			continue
		}
		open = append(open, t)
	}
	return AssembleTasks(records, meta, now), open
}

// withCallStates says for each subagent call of msgs whether its subagent runs,
// has finished or has no record. It decides from records and not from the task
// rows, which hold only the open Task group. msgs and their SubagentCalls share
// memory with the cache, which must hold no verdict, so both are copied.
func withCallStates(msgs []wire.SessionChatMessage, records []transcript.TaskRecord, meta Meta, now time.Time) []wire.SessionChatMessage {
	out := slices.Clone(msgs)
	for i := range out {
		calls := slices.Clone(out[i].SubagentCalls)
		for j := range calls {
			state := wire.SubagentCallNoRecord
			if rec, ok := recordOfCall(records, calls[j].CallID); ok {
				state = wire.SubagentCallRunning
				if resolveTask(rec, meta, now).finished {
					state = wire.SubagentCallFinished
				}
			}
			calls[j].State = state
		}
		out[i].SubagentCalls = calls
	}
	return out
}
