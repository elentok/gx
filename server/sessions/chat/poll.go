package chat

import (
	"os"
	"sync"
	"time"

	"github.com/elentok/gx/server/wire"
)

// Poll publishes EventSessionChatChanged when the session's transcript
// size changes. It ticks only between SetActive(true) and SetActive(false), so
// an Idle or Exited session costs nothing.
type Poll struct {
	c    *Chat
	dir  string
	id   string
	mu   sync.Mutex
	stop chan struct{} // non-nil while ticking
	done chan struct{}
}

// NewPoll makes the poll of the session id whose state dir is dir.
func (c *Chat) NewPoll(dir, id string) *Poll {
	return &Poll{c: c, dir: dir, id: id}
}

func (p *Poll) SetActive(active bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case active && p.stop == nil:
		p.stop, p.done = make(chan struct{}), make(chan struct{})
		// The baseline is taken here, not at the first tick: the page loads the
		// chat when the turn starts, and growth after that load is news.
		path := p.transcript()
		go p.run(p.stop, p.done, path, sizeOf(path))
	case !active && p.stop != nil:
		p.halt()
	}
}

func (p *Poll) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stop != nil {
		p.halt()
	}
}

// halt stops the ticking goroutine and waits for it. The caller holds p.mu.
func (p *Poll) halt() {
	close(p.stop)
	<-p.done
	p.stop, p.done = nil, nil
}

// sizeOf is the file's size, or -1 when it cannot be read.
func sizeOf(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}

func (p *Poll) run(stop <-chan struct{}, done chan<- struct{}, path string, size int64) {
	defer close(done)
	ticker := time.NewTicker(p.c.cfg.ChatPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-p.c.src.Stopped():
			return
		case <-ticker.C:
		}
		// A stall is only time passing, so no status change reports it.
		p.c.src.NoteFailure(p.id)
		p.c.src.NoteCards(p.id)
		// /clear starts a new conversation with a new transcript, so the path is
		// resolved every tick. A moved path is a change the page must refetch for:
		// its last-message id belongs to the old file. A transcript that appears
		// is news too.
		cur := p.transcript()
		cursize := sizeOf(cur)
		// An unreadable file at the same path keeps the last size.
		if cur == path && (cursize < 0 || cursize == size) {
			continue
		}
		path, size = cur, cursize
		p.c.src.Publish(wire.EventSessionChatChanged, p.id)
	}
}

func (p *Poll) transcript() string {
	meta, err := p.c.src.ReadMeta(p.dir)
	if err != nil || !p.c.src.HasChatReader(meta.Harness) {
		return ""
	}
	path, _, _ := p.c.Transcript(p.id, meta)
	return path
}
