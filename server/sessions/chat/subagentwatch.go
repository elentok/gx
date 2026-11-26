package chat

import (
	"os"
	"sync"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

// SubagentWatch ticks while a page shows subagent work: one subagent page, or a
// session's working card. It stats the files on the chat poll interval and calls
// publish when a size or mtime moves; it never parses a subagent file.
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
		// Each tick re-judges the call: a finished one is no longer watched, and
		// a Starting one keeps looking until its file appears.
		t = resolveSubagent(c, id, dir, callID)
		if !t.known || !t.live {
			return
		}
	}
}

// WatchSession watches the files of the running subagents of session id. With
// one running it calls publish before it returns, after it recorded the files;
// then once per tick that finds a file grown or new. It goes on while nothing
// runs, so a later subagent is picked up.
func WatchSession(c *Chat, id, dir string, publish func()) *SubagentWatch {
	w := &SubagentWatch{stop: make(chan struct{}), done: make(chan struct{})}
	prev, running, _ := runningSubagentFiles(c, id, dir)
	if running {
		publish()
	}
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(c.cfg.ChatPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-c.src.Stopped():
				return
			case <-ticker.C:
			}
			// A failed read keeps the old stamps: dropping them would make every
			// file look new on the next tick.
			cur, _, ok := runningSubagentFiles(c, id, dir)
			if !ok {
				continue
			}
			for path, stamp := range cur {
				if old, seen := prev[path]; !seen || old != stamp {
					publish()
					break
				}
			}
			prev = cur
		}
	}()
	return w
}

// runningSubagentFiles stamps the files found for the session's running
// subagents; running also counts one whose file is not there yet. ok is false
// when the parent transcript cannot be read.
//
// It judges a call by the parent transcript alone: resolvedCall.live also reads
// the sidecar's finished key, and filling the records would parse the files.
func runningSubagentFiles(c *Chat, id, dir string) (files map[string]fileStamp, running, ok bool) {
	meta, err := c.src.ReadMeta(dir)
	if err != nil {
		return nil, false, false
	}
	path, kind, found := c.Transcript(id, meta)
	if !found {
		return nil, false, false
	}
	parent, err := c.ReadSnapshot(path, kind, meta.Flow, dir)
	if err != nil {
		return nil, false, false
	}
	files = map[string]fileStamp{}
	for _, rec := range parent.Records {
		if rec.Kind != transcript.TaskKindSubagent || resolveTask(rec, meta, c.clock()).finished {
			continue
		}
		running = true
		var ref subagentRef
		var has bool
		switch kind {
		case harness.Codex:
			c.transcripts.ForgetMiss(rec.ID)
			ref, has = c.codexSubagent(rec, meta)
		case harness.Claude:
			ref, has = c.claudeSubagent(rec, path)
		}
		if !has {
			continue
		}
		if stamp := stampOf(ref.Path); stamp != nil {
			files[ref.Path] = *stamp
		}
	}
	return files, running, true
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
	rc := resolveCall(c, kind, path, meta, parent.Records, callID)
	if !rc.inParent {
		return subagentTarget{}
	}
	t := subagentTarget{known: true, live: rc.live(c, meta)}
	if rc.hasRef {
		t.path = existingFile(rc.ref.Path)
	}
	return t
}

func existingFile(path string) string {
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}
