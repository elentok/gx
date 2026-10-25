package server

import (
	"os"
	"sync"
	"time"
)

// defaultTranscriptMissTTL is how long a failed transcript lookup stays
// remembered. A transcript not written yet shows up within a few seconds.
const defaultTranscriptMissTTL = 3 * time.Second

// transcriptPaths remembers where each conversation's transcript is. A found
// path never moves, so a lookup globs the projects dir at most once per
// conversation, and once per transcriptMissTTL while it stays missing.
type transcriptPaths struct {
	mu     sync.Mutex
	found  map[string]string
	missed map[string]time.Time
}

// resolve returns the cached path of conversationID, or the result of find,
// which runs under the lock so concurrent callers share one glob. A miss
// stays cached for missTTL.
func (c *transcriptPaths) resolve(conversationID string, now time.Time, missTTL time.Duration, find func() (string, bool)) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if path, ok := c.found[conversationID]; ok {
		return path, true
	}
	if at, ok := c.missed[conversationID]; ok && now.Sub(at) < missTTL {
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
		c.missed = map[string]time.Time{}
	}
	c.missed[conversationID] = now
	return "", false
}

// authFailures keeps whether each transcript ends in an auth error until its
// size or mtime changes, so listing a quiet session never rereads the file.
type authFailures struct {
	mu      sync.Mutex
	entries map[string]authFailureEntry
}

type authFailureEntry struct {
	size  int64
	mtime time.Time
	auth  bool
}

func (c *authFailures) read(path string, check func(string) (bool, error)) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[path]; ok && e.size == info.Size() && e.mtime.Equal(info.ModTime()) {
		return e.auth, nil
	}
	auth, err := check(path)
	if err != nil {
		return false, err
	}
	if c.entries == nil {
		c.entries = map[string]authFailureEntry{}
	}
	c.entries[path] = authFailureEntry{size: info.Size(), mtime: info.ModTime(), auth: auth}
	return auth, nil
}
