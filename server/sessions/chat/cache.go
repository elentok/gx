package chat

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/chatreader"
	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

// maxChatMessages caps how many of the newest visible messages the chat loads.
const maxChatMessages = 300

// Cache keeps each transcript's parsed chat until its size or mtime
// changes, so polling a quiet session never rereads the file. The design's
// final-step flag and the harness are part of the key: a step change alone
// must refresh Next, and the same path read as another harness builds a
// different Next.
type Cache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	size  int64
	mtime time.Time
	kind  agentrunner.Kind
	final bool
	chat  Snapshot
}

// Snapshot is everything one read of a transcript yields. Tasks keeps the
// unresolved markers, which age out at request time.
type Snapshot struct {
	Msgs      []wire.SessionChatMessage
	Truncated bool
	Round     []wire.SessionRoundCard
	Next      *wire.SessionNext
	Tasks     []transcript.BackgroundTaskMarker
	// Unreadable and CodexVersion are set for a Codex rollout gx cannot read.
	Unreadable   bool
	CodexVersion string
}

// Read returns the cached snapshot of path, or load's result when the file's
// size or mtime, the harness kind or the final flag changed.
func (c *Cache) Read(path string, kind agentrunner.Kind, final bool, load func(string) (Snapshot, error)) (Snapshot, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Snapshot{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[path]; ok && e.size == info.Size() && e.mtime.Equal(info.ModTime()) && e.kind == kind && e.final == final {
		return e.chat, nil
	}
	chat, err := load(path)
	if err != nil {
		return Snapshot{}, err
	}
	if c.entries == nil {
		c.entries = map[string]cacheEntry{}
	}
	c.entries[path] = cacheEntry{size: info.Size(), mtime: info.ModTime(), kind: kind, final: final, chat: chat}
	return chat, nil
}

// ImagesDir is the folder of a session's Prompt images, inside its state dir.
const ImagesDir = "images"

// Loader returns the load func for a session whose design ticket is ticket;
// final is whether its map is at a step with no Continue. sessionDir is the
// session's state dir, whose images folder the chat's saved images sit in.
func Loader(kind agentrunner.Kind, ticket, sessionDir string, final bool) func(string) (Snapshot, error) {
	return func(path string) (Snapshot, error) {
		read, _, err := chatreader.ReadFile(kind, path, transcript.WithImagesDir(filepath.Join(sessionDir, ImagesDir)))
		if err != nil {
			return Snapshot{}, err
		}
		snap := SnapshotOf(read, kind, ticket, final)
		// Codex has no background markers: its Background flag comes from the status.
		if kind == harness.Codex {
			return snap, nil
		}
		// A zero now keeps every unresolved marker fresh; the cache serves the
		// read later, so ageing out waits for the request.
		reading, err := transcript.ReadBackgroundTasks(path, transcript.BackgroundTaskAgedOutCap, time.Time{})
		if err != nil {
			return Snapshot{}, err
		}
		for _, mk := range reading.Markers {
			if mk.Status != transcript.BackgroundTaskResolved {
				snap.Tasks = append(snap.Tasks, mk)
			}
		}
		return snap, nil
	}
}

// SnapshotOf turns one read of a conversation into what the chat serves.
func SnapshotOf(read chatreader.Result, kind agentrunner.Kind, ticket string, final bool) Snapshot {
	loaded := read.Messages
	// Read before the cap and the fence strip: both cut what the parsers need.
	// An open round wins over the run-end line, as in designActionable.
	snap := Snapshot{Round: chatRound(loaded), Unreadable: read.Unreadable, CodexVersion: read.CodexVersion}
	if snap.Round == nil {
		snap.Next = ChatNext(loaded, kind, ticket, final)
	}
	if len(loaded) > maxChatMessages {
		loaded, snap.Truncated = loaded[len(loaded)-maxChatMessages:], true
	}
	snap.Msgs = make([]wire.SessionChatMessage, 0, len(loaded))
	for _, m := range loaded {
		text := m.Text
		if m.Role == transcript.ChatRoleAgent && m.Kind == transcript.ChatKindText {
			text, _ = grillround.SplitFences(text)
		}
		var images []wire.SessionChatImage
		for _, im := range m.Images {
			images = append(images, wire.SessionChatImage{ID: im.ID, Placeholder: im.Placeholder})
		}
		snap.Msgs = append(snap.Msgs, wire.SessionChatMessage{ID: m.ID, Role: m.Role, Kind: m.Kind, Text: text, At: m.At, ToolCount: m.ToolCount, LastTool: m.LastTool, Calls: m.Calls, Images: images})
	}
	return snap
}

// Background lists the tasks still within their cap at now.
func (c Snapshot) Background(now time.Time) []wire.SessionBackgroundTask {
	out := []wire.SessionBackgroundTask{}
	for _, mk := range c.Tasks {
		if now.Sub(mk.StartedAt) <= mk.Cap {
			out = append(out, wire.SessionBackgroundTask{Kind: mk.Kind, Label: mk.Label, StartedAt: mk.StartedAt.Format(time.RFC3339)})
		}
	}
	return out
}
