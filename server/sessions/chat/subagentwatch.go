package chat

import (
	"os"
	"sync"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

// SubagentWatch ticks while one subagent page is open. It stats the file on the
// chat poll interval and calls publish when its size or mtime moves; it never
// parses the file.
type SubagentWatch struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// Close ends the watch and waits for its goroutine.
func (w *SubagentWatch) Close() {
	w.once.Do(func() { close(w.stop) })
	<-w.done
}

// subagentTarget is what one resolution of a call finds.
type subagentTarget struct {
	known bool   // the call is in the session
	live  bool   // the call still runs: a finished one is not watched
	path  string // the subagent file, empty while it is not found
}

// WatchSubagent opens a watch on the subagent call of session id. known is
// false for a call the session does not hold. A nil watch with known true means
// there is nothing to watch: the call is over or its page is Not available.
func WatchSubagent(c *Chat, id, dir, callID string, publish func()) (w *SubagentWatch, known bool) {
	t := resolveSubagent(c, id, dir, callID)
	if !t.known {
		return nil, false
	}
	if !t.live {
		return nil, true
	}
	w = &SubagentWatch{stop: make(chan struct{}), done: make(chan struct{})}
	// A file already there is the baseline the page loaded, taken before the
	// goroutine starts; one that appears later is news.
	var prev *fileStamp
	if t.path != "" {
		prev = stampOf(t.path)
	}
	go watchSubagent(c, w, id, dir, callID, t, prev, publish)
	return w, true
}

type fileStamp struct {
	size int64
	mod  time.Time
}

func stampOf(path string) *fileStamp {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return &fileStamp{info.Size(), info.ModTime()}
}

func watchSubagent(c *Chat, w *SubagentWatch, id, dir, callID string, t subagentTarget, prev *fileStamp, publish func()) {
	defer close(w.done)
	ticker := time.NewTicker(c.cfg.ChatPollInterval)
	defer ticker.Stop()
	for {
		if t.path != "" {
			if cur := stampOf(t.path); cur == nil {
				t.path, prev = "", nil
			} else {
				if prev == nil || *prev != *cur {
					publish()
				}
				prev = cur
			}
		}
		select {
		case <-w.stop:
			return
		case <-c.src.Stopped():
			return
		case <-ticker.C:
		}
		if t.path == "" {
			// A Starting call's file is found by lookups that remember misses.
			t = resolveSubagent(c, id, dir, callID)
		}
	}
}

// resolveSubagent resolves the call the way a chat read with subagent= does. A
// Codex miss is forgotten first: the lookup runs on a tick, not on an event.
func resolveSubagent(c *Chat, id, dir, callID string) subagentTarget {
	meta, err := c.src.ReadMeta(dir)
	if err != nil {
		return subagentTarget{known: true}
	}
	path, kind, found := c.Transcript(id, meta)
	if !found {
		return subagentTarget{known: true}
	}
	parent, err := c.ReadSnapshot(path, kind, meta.Flow, dir)
	if err != nil {
		return subagentTarget{known: true}
	}
	rec, inParent := recordOfCall(parent.Records, callID)
	switch kind {
	case harness.Codex:
		if !inParent {
			return subagentTarget{}
		}
		c.transcripts.ForgetMiss(rec.ID)
		rec = c.withCodexChildren([]transcript.TaskRecord{rec}, meta)[0]
		t := subagentTarget{known: true, live: !resolveTask(rec, meta, c.clock()).finished}
		if ref, ok := c.codexSubagent(rec, meta); ok {
			t.path = existingFile(ref.Path)
		}
		return t
	case harness.Claude:
		if !inParent {
			rec = transcript.TaskRecord{Kind: transcript.TaskKindSubagent, CallID: callID}
		}
		ref, ok := c.claudeSubagent(rec, path)
		if ok && !inParent {
			rec, _, inParent = c.nestedCall(parent.Records, path, rec, ref)
		}
		if !inParent {
			return subagentTarget{}
		}
		rec = c.withSubagentDetails([]transcript.TaskRecord{rec}, path)[0]
		t := subagentTarget{known: true, live: !resolveTask(rec, meta, c.clock()).finished && !ref.Finished}
		if ok {
			t.path = existingFile(ref.Path)
		}
		return t
	}
	return subagentTarget{}
}

func existingFile(path string) string {
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}
