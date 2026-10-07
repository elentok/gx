package tickets

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Seam D: refresh mode follows the connection.
func TestRefreshMode_FollowsConnection(t *testing.T) {
	m := newServerModel(t)
	if got := m.refreshMode(); got != refreshStream {
		t.Fatalf("connected mode = %v, want stream", got)
	}

	next, cmd := m.Update(ServerDownMsg{})
	m = next.(Model)
	if got := m.refreshMode(); got != refreshWatchPoll {
		t.Fatalf("down mode = %v, want watch+poll", got)
	}
	if cmd == nil {
		t.Fatal("going down must start the fallback watch and poll")
	}

	next, cmd = m.Update(ServerUpMsg{})
	m = next.(Model)
	if got := m.refreshMode(); got != refreshStream {
		t.Fatalf("reconnected mode = %v, want stream", got)
	}
	if cmd == nil {
		t.Fatal("reconnecting must re-snapshot")
	}
}

func TestRefreshMode_NoDiskPollWhileConnected(t *testing.T) {
	m := newServerModel(t)
	next, cmd := m.Update(epicsLoadedMsg{})
	if cmd != nil {
		t.Fatalf("a connected load scheduled %v; want no poll timer", cmd)
	}
	_ = next
}

func TestRefreshMode_DownFallbackPollsAndIgnoresStaleTicks(t *testing.T) {
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

func TestRefreshMode_DownWatchFiresOnStoreChange(t *testing.T) {
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
