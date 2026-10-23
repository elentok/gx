package server

import (
	"os"
	"sync"
	"time"
)

// EventSessionChatChanged announces that a working session's transcript grew
// or shrank; its address is the session id.
const EventSessionChatChanged = "session-chat-changed"

const defaultChatPollInterval = 2 * time.Second

// chatPoll publishes EventSessionChatChanged when the session's transcript
// size changes. It ticks only between setActive(true) and setActive(false), so
// an Idle or Exited session costs nothing.
type chatPoll struct {
	s    *Server
	dir  string
	id   string
	mu   sync.Mutex
	stop chan struct{} // non-nil while ticking
	done chan struct{}
}

func (s *Server) newChatPoll(dir, id string) *chatPoll {
	return &chatPoll{s: s, dir: dir, id: id}
}

func (p *chatPoll) setActive(active bool) {
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

func (p *chatPoll) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stop != nil {
		p.halt()
	}
}

// halt stops the ticking goroutine and waits for it. The caller holds p.mu.
func (p *chatPoll) halt() {
	close(p.stop)
	<-p.done
	p.stop, p.done = nil, nil
}

func (p *chatPoll) run(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(p.s.cfg.ChatPollInterval)
	defer ticker.Stop()
	var path string
	size := int64(-1)
	for {
		select {
		case <-stop:
			return
		case <-p.s.sessionWatches.stop:
			return
		case <-ticker.C:
		}
		// A stall is only time passing, so no status change reports it.
		p.s.noteFailure(p.id)
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
			p.s.events.publish(EventSessionChatChanged, p.id)
		}
	}
}

func (p *chatPoll) transcript() string {
	meta, err := readSessionMeta(p.dir)
	if err != nil || meta.Harness != p.s.settings.Load().chatHarness() {
		return ""
	}
	path, _ := p.s.claudeTranscript(p.id, meta.ConversationID)
	return path
}
