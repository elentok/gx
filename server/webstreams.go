package server

import (
	"context"
	"sync"
)

// webStreams tracks the open event streams by web login id, so signing a web
// login out can end its streams. The zero value is ready to use.
type webStreams struct {
	mu     sync.Mutex
	next   int
	cancel map[string]map[int]context.CancelFunc
}

// track returns a context that ends with ctx or when the login's streams are
// closed, and the function that stops tracking it.
func (ws *webStreams) track(ctx context.Context, loginID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.cancel == nil {
		ws.cancel = map[string]map[int]context.CancelFunc{}
	}
	if ws.cancel[loginID] == nil {
		ws.cancel[loginID] = map[int]context.CancelFunc{}
	}
	key := ws.next
	ws.next++
	ws.cancel[loginID][key] = cancel
	return ctx, func() {
		ws.mu.Lock()
		defer ws.mu.Unlock()
		delete(ws.cancel[loginID], key)
		if len(ws.cancel[loginID]) == 0 {
			delete(ws.cancel, loginID)
		}
		cancel()
	}
}

// closeLogins ends every open stream of the given web logins.
func (ws *webStreams) closeLogins(ids ...string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	for _, id := range ids {
		for _, cancel := range ws.cancel[id] {
			cancel()
		}
	}
}
