package chat

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/chatreader"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
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
	snap := SnapshotOf(read, harness.Codex, meta.Ticket, c.src.DesignFinal(meta.Ticket))
	wire.WriteJSON(w, wire.SessionChatResponse{
		ConversationID: meta.ConversationID, Messages: snap.Msgs, Truncated: snap.Truncated,
		Round: snap.Round, Next: snap.Next, Background: []wire.SessionBackgroundTask{},
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
	out := wire.SessionChatResponse{ConversationID: meta.ConversationID, Messages: []wire.SessionChatMessage{}, Background: []wire.SessionBackgroundTask{}, Failure: c.Failure(meta)}
	if !c.src.HasChatReader(meta.Harness) {
		out.Unsupported = true
		wire.WriteJSON(w, out)
		return
	}
	// The page reads only on an event, so a remembered miss from before the
	// transcript existed would hide the message the event announces.
	c.transcripts.ForgetMiss(meta.ConversationID)
	path, kind, found := c.Transcript(id, meta)
	if !found {
		_, out.Archived = c.codexArchive(id, meta)
	} else {
		snap, err := c.ReadSnapshot(path, kind, meta.Ticket, dir)
		switch {
		case os.IsNotExist(err): // vanished since the lookup: no chat yet
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		default:
			out.Messages, out.Truncated = snap.Msgs, snap.Truncated
			out.Round, out.Next, out.Background = snap.Round, snap.Next, snap.Background(time.Now())
			out.Unreadable, out.CodexVersion = snap.Unreadable, snap.CodexVersion
		}
	}
	wire.WriteJSON(w, out)
}
