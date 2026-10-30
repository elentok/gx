package server

import (
	"cmp"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/elentok/gx/grillround"
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
// says earlier messages were cut to fit maxChatMessages. Round holds the cards
// of a Grilling round the agent ended its last turn with and the person has not
// answered; Next is the run-end line of that turn; Background lists the shells,
// Monitors and subagents still running.
type SessionChatResponse struct {
	ConversationID string                  `json:"conversation_id"`
	Messages       []SessionChatMessage    `json:"messages"`
	Unsupported    bool                    `json:"unsupported,omitempty"`
	Truncated      bool                    `json:"truncated,omitempty"`
	Round          []SessionRoundCard      `json:"round" tstype:"SessionRoundCard[] | null"`
	Next           *SessionNext            `json:"next"`
	Background     []SessionBackgroundTask `json:"background"`
	// Failure is the session's, as on its SessionInfo.
	Failure *SessionFailure `json:"failure,omitempty"`
}

// chatCache keeps each transcript's parsed chat until its size or mtime
// changes, so polling a quiet session never rereads the file. The design's
// final-step flag is part of the key: a step change alone must refresh Next.
type chatCache struct {
	mu      sync.Mutex
	entries map[string]chatCacheEntry
}

type chatCacheEntry struct {
	size  int64
	mtime time.Time
	final bool
	chat  chatSnapshot
}

// chatSnapshot is everything one read of a transcript yields. Tasks keeps the
// unresolved markers, which age out at request time.
type chatSnapshot struct {
	msgs      []SessionChatMessage
	truncated bool
	round     []SessionRoundCard
	next      *SessionNext
	tasks     []transcript.BackgroundTaskMarker
}

func (c *chatCache) read(path string, final bool, load func(string) (chatSnapshot, error)) (chatSnapshot, error) {
	info, err := os.Stat(path)
	if err != nil {
		return chatSnapshot{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[path]; ok && e.size == info.Size() && e.mtime.Equal(info.ModTime()) && e.final == final {
		return e.chat, nil
	}
	chat, err := load(path)
	if err != nil {
		return chatSnapshot{}, err
	}
	if c.entries == nil {
		c.entries = map[string]chatCacheEntry{}
	}
	c.entries[path] = chatCacheEntry{size: info.Size(), mtime: info.ModTime(), final: final, chat: chat}
	return chat, nil
}

// chatLoader returns the load func for a session whose design ticket is ticket;
// final is whether its map is at a step with no Continue.
func chatLoader(ticket string, final bool) func(string) (chatSnapshot, error) {
	return func(path string) (chatSnapshot, error) {
		loaded, _, err := transcript.ReadChat(path)
		if err != nil {
			return chatSnapshot{}, err
		}
		// Read before the cap and the fence strip: both cut what the parsers need.
		// An open round wins over the run-end line, as in designActionable.
		snap := chatSnapshot{round: chatRound(loaded)}
		if snap.round == nil {
			snap.next = chatNext(loaded, ticket, final)
		}
		if len(loaded) > maxChatMessages {
			loaded, snap.truncated = loaded[len(loaded)-maxChatMessages:], true
		}
		snap.msgs = make([]SessionChatMessage, 0, len(loaded))
		for _, m := range loaded {
			text := m.Text
			if m.Role == transcript.ChatRoleAgent {
				text, _ = grillround.SplitFences(text)
			}
			snap.msgs = append(snap.msgs, SessionChatMessage{ID: m.ID, Role: m.Role, Kind: m.Kind, Text: text, At: m.At, ToolCount: m.ToolCount, LastTool: m.LastTool})
		}
		// A zero now keeps every unresolved marker fresh; the cache serves the
		// read later, so ageing out waits for the request.
		reading, err := transcript.ReadBackgroundTasks(path, transcript.BackgroundTaskAgedOutCap, time.Time{})
		if err != nil {
			return chatSnapshot{}, err
		}
		for _, mk := range reading.Markers {
			if mk.Status != transcript.BackgroundTaskResolved {
				snap.tasks = append(snap.tasks, mk)
			}
		}
		return snap, nil
	}
}

// background lists the tasks still within their cap at now.
func (c chatSnapshot) background(now time.Time) []SessionBackgroundTask {
	out := []SessionBackgroundTask{}
	for _, mk := range c.tasks {
		if now.Sub(mk.StartedAt) <= mk.Cap {
			out = append(out, SessionBackgroundTask{Kind: mk.Kind, Label: mk.Label, StartedAt: mk.StartedAt.Format(time.RFC3339)})
		}
	}
	return out
}

// chatHarness is the harness whose transcripts the chat reads.
func (s Settings) chatHarness() string {
	return cmp.Or(s.ChatHarness, string(harness.Claude))
}

func (s *Server) sessionChatRead(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session")
	_, meta, res, err := s.readableSession(id)
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
	out := SessionChatResponse{ConversationID: meta.ConversationID, Messages: []SessionChatMessage{}, Background: []SessionBackgroundTask{}, Failure: s.sessionInfo(meta).Failure}
	if meta.Harness != s.settings.Load().chatHarness() {
		out.Unsupported = true
		writeJSON(w, out)
		return
	}
	// The page reads only on an event, so a remembered miss from before the
	// transcript existed would hide the message the event announces.
	s.transcripts.forgetMiss(meta.ConversationID)
	if path, ok := s.claudeTranscript(id, meta); ok {
		final := s.designFinal(meta.Ticket)
		chat, err := s.chats.read(path, final, chatLoader(meta.Ticket, final))
		switch {
		case os.IsNotExist(err): // vanished since the lookup: no chat yet
		case err != nil:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		default:
			out.Messages, out.Truncated = chat.msgs, chat.truncated
			out.Round, out.Next, out.Background = chat.round, chat.next, chat.background(time.Now())
		}
	}
	writeJSON(w, out)
}
