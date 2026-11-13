package prompt

import "sync"

// typingLocks is one lock per chat, keyed by session id. A typer holds it for
// one clear, type and Enter, never for a turn, so two typers never interleave
// their keys in one input box. It is a field of Prompt and lockTyping a plain
// function, so Prompt's method count stays under the sizeguard limit.
type typingLocks struct {
	mu    sync.Mutex
	chats map[string]chan struct{}
}

// lockTyping waits for the chat's typing lock. It gives up with
// ErrServerStopping when the server stops and with errNudgeCancelled when
// cancel closes. The returned func releases the lock.
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

func (l *typingLocks) slot(id string) chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.chats == nil {
		l.chats = map[string]chan struct{}{}
	}
	ch, ok := l.chats[id]
	if !ok {
		ch = make(chan struct{}, 1)
		l.chats[id] = ch
	}
	return ch
}
