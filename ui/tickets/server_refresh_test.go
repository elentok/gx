package tickets

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// Seam D: refresh mode follows the connection.
func TestRefresh_FollowsConnection(t *testing.T) {
	m := newServerModel(t)
	if m.onFallback() {
		t.Fatal("connected tab is on the fallback")
	}

	next, cmd := m.Update(ServerDownMsg{})
	m = next.(Model)
	if !m.onFallback() {
		t.Fatal("down tab is not on the fallback")
	}
	if cmd == nil {
		t.Fatal("going down must start the fallback watch and poll")
	}

	// The shell re-snapshots on reconnect and delivers; the tab fetches nothing.
	next, cmd = m.Update(ServerUpMsg{})
	m = next.(Model)
	if m.onFallback() {
		t.Fatal("reconnected tab is on the fallback")
	}
	if cmd != nil {
		t.Fatal("reconnecting must not fetch a snapshot itself")
	}
}

// Seam B: with no client the tab starts, and stays, on the down fallback.
func TestRefresh_NoClientStartsDown(t *testing.T) {
	m := NewModel(t.TempDir(), ui.Settings{}, keys.Manager{})
	next, _ := m.Update(m.Init()())
	m = next.(Model)
	t.Cleanup(m.fallbackStop)
	if !m.onFallback() || m.serverLink != ServerLinkDown {
		t.Fatalf("fallback=%v link=%v, want the down fallback", m.onFallback(), m.serverLink)
	}
	if _, blocked := m.serverKeyGuard(bindingTicketsAddToQueue); !blocked {
		t.Fatal(`"a" is not blocked without a server`)
	}
}

func TestRefresh_NoDiskPollWhileConnected(t *testing.T) {
	m := newServerModel(t)
	next, cmd := m.Update(epicsLoadedMsg{})
	if cmd != nil {
		t.Fatalf("a connected load scheduled %v; want no poll timer", cmd)
	}
	_ = next
}

func TestRefresh_DownFallbackPollsAndIgnoresStaleTicks(t *testing.T) {
	m := newServerModel(t)
	next, _ := m.Update(ServerDownMsg{})
	m = next.(Model)

	_, cmd := m.Update(fallbackPollMsg{gen: m.fallbackGen})
	if cmd == nil {
		t.Fatal("a live poll tick must reload and re-arm")
	}

	next, _ = m.Update(ServerUpMsg{})
	m = next.(Model)
	_, cmd = m.Update(fallbackPollMsg{gen: m.fallbackGen - 1})
	if cmd != nil {
		t.Fatal("a tick from a finished fallback must be dropped")
	}
}

func TestRefresh_DownWatchFiresOnStoreChange(t *testing.T) {
	dir := t.TempDir()
	events, stop, err := watchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if err := os.WriteFile(filepath.Join(dir, "x.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-events:
	case <-time.After(5 * time.Second):
		t.Fatal("no change signal after a store write")
	}
}

var _ tea.Msg = fallbackPollMsg{}
