package app

import (
	"io"
	"path/filepath"
	"sync"
	"testing"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/agents/terminal"
	"github.com/elentok/gx/ui/nav"
)

// idleAttach is an attach stream with nothing to say: Next blocks until Close.
type idleAttach struct {
	done chan struct{}
	once sync.Once
}

func (a *idleAttach) Next() (holder.AttachMessage, error) {
	<-a.done
	return holder.AttachMessage{}, io.EOF
}
func (a *idleAttach) SendKey(screen.Key) error                    { return nil }
func (a *idleAttach) Paste(string) error                          { return nil }
func (a *idleAttach) Scroll(int) error                            { return nil }
func (a *idleAttach) Wheel(int, int, bool) error                  { return nil }
func (a *idleAttach) Mouse(int, int, screen.MouseKind, int) error { return nil }
func (a *idleAttach) Copy(holder.LinePos, holder.LinePos) error   { return nil }
func (a *idleAttach) Resize(int, int) error                       { return nil }
func (a *idleAttach) TakeOver() error                             { return nil }
func (a *idleAttach) Close() error                                { a.once.Do(func() { close(a.done) }); return nil }

// TestServerStream_AttachesWhenSwitchedToAfterTheStateLoaded: the page's dial
// command must survive the tab switch, or the Agents tab never attaches to a
// session that was already on screen.
func TestServerStream_AttachesWhenSwitchedToAfterTheStateLoaded(t *testing.T) {
	h, f := newStreamHarness(t)
	f.mu.Lock()
	f.sessions = []wire.SessionInfo{{ID: "s1", Name: "alpha", Status: wire.SessionStatus{State: wire.SessionIdle}}}
	f.mu.Unlock()

	var mu sync.Mutex
	var dirs []string
	h.m.settings.Server.AttachDialer = func(dir string) (terminal.AttachStream, []string, error) {
		mu.Lock()
		dirs = append(dirs, dir)
		mu.Unlock()
		a := &idleAttach{done: make(chan struct{})}
		t.Cleanup(func() { _ = a.Close() })
		return a, nil, nil
	}

	h.probeUp()
	h.switchTo(nav.TabAgents)
	h.pump()

	mu.Lock()
	defer mu.Unlock()
	if len(dirs) == 0 || filepath.Base(dirs[0]) != "s1" {
		t.Fatalf("dialed %v, want the directory of session s1", dirs)
	}
}
