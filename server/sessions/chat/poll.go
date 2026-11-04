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
		go p.run(p.stop, p.done)
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

func (p *Poll) run(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(p.c.cfg.ChatPollInterval)
	defer ticker.Stop()
	var path string
	size := int64(-1)
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
		if path == "" {
			path = p.transcript()
		}
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		prev := size
		size = info.Size()
		// The first reading is the baseline: growth that predates the poll
		// is already in the chat the page loads.
		if prev >= 0 && size != prev {
			p.c.src.Publish(wire.EventSessionChatChanged, p.id)
		}
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
