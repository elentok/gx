package prompt

import "sync"

// typingLocks is one lock per chat, held for one clear, type and Enter, so two
// typers never interleave keys. Forgetting a chat is safe mid-typing.
type typingLocks struct {
	mu    sync.Mutex
	chats map[string]chan struct{}
}

// ForgetSession drops a gone session's typing lock and rename slot.
func ForgetSession(p *Prompt, id string) {
	p.typingLocks.forget(id)
	p.renames.forget(id)
}

// lockTyping waits for the chat's lock. The returned func releases it.
func lockTyping(p *Prompt, id string, cancel <-chan struct{}) (func(), error) {
	ch := p.typingLocks.slot(id)
	var stop <-chan struct{}
	if p.sessions != nil {
		stop = p.sessions.Watches.Stop
	}
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	case <-cancel:
		return nil, errNudgeCancelled
	case <-stop:
		return nil, ErrServerStopping
	}
}

func (l *typingLocks) forget(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.chats, id)
}

func (l *typingLocks) slot(id string) chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.chats == nil {
		l.chats = map[string]chan struct{}{}
	}
	if l.chats[id] == nil {
		l.chats[id] = make(chan struct{}, 1)
	}
	return l.chats[id]
}
