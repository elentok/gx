package server

import (
	"cmp"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

// SessionChatMessage is one visible message of a session's conversation. ID is
// the transcript line's uuid.
type SessionChatMessage struct {
	ID   string    `json:"id"`
	Role string    `json:"role"` // transcript.ChatRoleAgent or ChatRolePerson
	Kind string    `json:"kind"` // text, prompt or command
	Text string    `json:"text"`
	At   time.Time `json:"at"`
	// ToolCount and LastTool describe a tools entry, which folds a run of tool
	// calls and carries no Text.
	ToolCount int    `json:"tool_count,omitempty"`
	LastTool  string `json:"last_tool,omitempty"`
}

// maxChatMessages caps how many of the newest visible messages the chat loads.
const maxChatMessages = 300

// SessionChatResponse is the visible messages of a session's newest
// conversation. ConversationID changes after a /clear. Unsupported marks a
// session of a harness that has no chat; its Messages stay empty. Truncated
// says earlier messages were cut to fit maxChatMessages.
type SessionChatResponse struct {
	ConversationID string               `json:"conversation_id"`
	Messages       []SessionChatMessage `json:"messages"`
	Unsupported    bool                 `json:"unsupported,omitempty"`
	Truncated      bool                 `json:"truncated,omitempty"`
}

// chatCache keeps each transcript's capped messages until its size or mtime
// changes, so polling a quiet session never rereads the file.
type chatCache struct {
	mu      sync.Mutex
	entries map[string]chatCacheEntry
}

type chatCacheEntry struct {
	size      int64
	mtime     time.Time
	msgs      []SessionChatMessage
	truncated bool
}

func (c *chatCache) read(path string, load func(string) ([]transcript.ChatMessage, error)) ([]SessionChatMessage, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[path]; ok && e.size == info.Size() && e.mtime.Equal(info.ModTime()) {
		return e.msgs, e.truncated, nil
	}
	loaded, err := load(path)
	if err != nil {
		return nil, false, err
	}
	e := chatCacheEntry{size: info.Size(), mtime: info.ModTime()}
	if len(loaded) > maxChatMessages {
		loaded, e.truncated = loaded[len(loaded)-maxChatMessages:], true
	}
	e.msgs = make([]SessionChatMessage, 0, len(loaded))
	for _, m := range loaded {
		e.msgs = append(e.msgs, SessionChatMessage{ID: m.ID, Role: m.Role, Kind: m.Kind, Text: m.Text, At: m.At, ToolCount: m.ToolCount, LastTool: m.LastTool})
	}
	if c.entries == nil {
		c.entries = map[string]chatCacheEntry{}
	}
	c.entries[path] = e
	return e.msgs, e.truncated, nil
}

func loadChat(path string) ([]transcript.ChatMessage, error) {
	msgs, _, err := transcript.ReadChat(path)
	return msgs, err
}

// chatHarness is the harness whose transcripts the chat reads.
func (s Settings) chatHarness() string {
	return cmp.Or(s.ChatHarness, string(harness.Claude))
}

func (s *Server) sessionChatRead(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session")
	_, meta, res, err := s.sessionDir(id)
	if res.Refused {
		status := http.StatusBadRequest
		if res.Reason == ReasonSessionUnknown {
			status = http.StatusNotFound
		}
		http.Error(w, res.Message, status)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := SessionChatResponse{ConversationID: meta.ConversationID, Messages: []SessionChatMessage{}}
	if meta.Harness != s.settings.Load().chatHarness() {
		out.Unsupported = true
		writeJSON(w, out)
		return
	}
	if path, ok := s.claudeTranscript(id, meta.ConversationID); ok {
		msgs, truncated, err := s.chats.read(path, loadChat)
		switch {
		case os.IsNotExist(err): // vanished since the lookup: no chat yet
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		default:
			out.Messages, out.Truncated = msgs, truncated
		}
	}
	writeJSON(w, out)
}
