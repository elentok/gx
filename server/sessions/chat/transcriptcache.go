// Package chat holds the parts of the session chat that need no Server: the
// transcript path and file caches, the parsed-chat cache, and the round and
// run-end parsers.
package chat

import (
	"os"
	"sync"
	"time"
)

// DefaultTranscriptMissTTL is how long the first failed transcript lookup stays
// remembered. A transcript not written yet shows up within a few seconds.
const DefaultTranscriptMissTTL = 3 * time.Second

// missBackoff scales the miss TTL on each repeated miss: a transcript still
// missing after a few tries is probably never coming (wrong harness, deleted),
// and every lookup of it costs a glob over the whole projects dir.
var missBackoff = []time.Duration{1, 10, 100}

// transcriptMiss is one remembered failed lookup. A session status change
// ends it early, since a new turn is when a transcript appears.
type transcriptMiss struct {
	at    time.Time
	state string
	tries int
}

// TranscriptPaths remembers where each conversation's transcript is. A found
// path never moves, so a lookup globs the projects dir at most once per
// conversation, and while it stays missing at most once per backed-off TTL.
type TranscriptPaths struct {
	mu     sync.Mutex
	found  map[string]string
	missed map[string]transcriptMiss
}

// Resolve returns the cached path of conversationID, or the result of find,
// which runs under the lock so concurrent callers share one glob. A miss
// stays cached for missTTL, then 10x and 100x that on repeated misses, and
// is dropped when state differs from the one it was recorded in.
func (c *TranscriptPaths) Resolve(conversationID, state string, now time.Time, missTTL time.Duration, find func() (string, bool)) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if path, ok := c.found[conversationID]; ok {
		return path, true
	}
	miss, missed := c.missed[conversationID]
	if missed && miss.state != state {
		missed = false
	}
	if missed && now.Sub(miss.at) < missTTL*missBackoff[min(miss.tries-1, len(missBackoff)-1)] {
		return "", false
	}
	path, ok := find()
	if ok {
		if c.found == nil {
			c.found = map[string]string{}
		}
		c.found[conversationID] = path
		delete(c.missed, conversationID)
		return path, true
	}
	if c.missed == nil {
		c.missed = map[string]transcriptMiss{}
	}
	tries := 1
	if missed {
		tries = miss.tries + 1
	}
	c.missed[conversationID] = transcriptMiss{at: now, state: state, tries: tries}
	return "", false
}

// ForgetMiss drops a remembered failed lookup, so the next Resolve globs again.
func (c *TranscriptPaths) ForgetMiss(conversationID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.missed, conversationID)
}

// FileCache keeps what Read computed from each file until its size or mtime
// changes, so listing a quiet session never rereads its transcript.
type FileCache[T any] struct {
	mu      sync.Mutex
	entries map[string]fileCacheEntry[T]
}

type fileCacheEntry[T any] struct {
	size  int64
	mtime time.Time
	value T
}

// Read returns the cached value for path, or compute's result when the file's
// size or mtime changed.
func (c *FileCache[T]) Read(path string, compute func(string) (T, error)) (T, error) {
	var zero T
	info, err := os.Stat(path)
	if err != nil {
		return zero, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[path]; ok && e.size == info.Size() && e.mtime.Equal(info.ModTime()) {
		return e.value, nil
	}
	value, err := compute(path)
	if err != nil {
		return zero, err
	}
	if c.entries == nil {
		c.entries = map[string]fileCacheEntry[T]{}
	}
	c.entries[path] = fileCacheEntry[T]{size: info.Size(), mtime: info.ModTime(), value: value}
	return value, nil
}

// AuthFailures caches whether each transcript ends in an auth error.
type AuthFailures = FileCache[bool]
