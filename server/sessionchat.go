package server

import (
	"cmp"
	"net/http"
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
}

// SessionChatResponse is the visible messages of a session's newest
// conversation. ConversationID changes after a /clear. Unsupported marks a
// session of a harness that has no chat; its Messages stay empty.
type SessionChatResponse struct {
	ConversationID string               `json:"conversation_id"`
	Messages       []SessionChatMessage `json:"messages"`
	Unsupported    bool                 `json:"unsupported,omitempty"`
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
		msgs, _, err := transcript.ReadChat(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, m := range msgs {
			out.Messages = append(out.Messages, SessionChatMessage{ID: m.ID, Role: m.Role, Kind: m.Kind, Text: m.Text, At: m.At})
		}
	}
	writeJSON(w, out)
}
