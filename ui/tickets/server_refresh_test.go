package tickets

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// Seam D: the tab reads the store only while the server is down.
func TestRefresh_FollowsConnection(t *testing.T) {
	m := newServerModel(t)
	if m.readsDisk() {
		t.Fatal("connected tab reads the store")
	}

	m, cmd := withLink(m, ServerLinkDown)
	if !m.readsDisk() {
		t.Fatal("down tab does not read the store")
	}
	if cmd == nil {
		t.Fatal("going down must load from disk")
	}
	if _, cmd = withLink(m, ServerLinkDown); cmd != nil {
		t.Fatal("a repeated down delivery must not reload")
	}

	// The shell re-snapshots on reconnect and delivers; the tab fetches nothing.
	m, cmd = withLink(m, ServerLinkUp)
	if m.readsDisk() {
		t.Fatal("reconnected tab reads the store")
	}
	if cmd != nil {
		t.Fatal("reconnecting must not fetch a snapshot itself")
	}
}

// Seam B: with no client the tab starts, and stays, reading the store.
func TestRefresh_NoClientStartsDown(t *testing.T) {
	// The shell delivers the down link when it has no client (ServerConn.link).
	m, _ := withLink(NewModel(t.TempDir(), ui.Settings{}, keys.Manager{}), ServerLinkDown)
	if m.Init() == nil {
		t.Fatal("a tab with no client must load from disk on Init")
	}
	if !m.readsDisk() {
		t.Fatal("a tab with no client does not read the store")
	}
	if _, blocked := m.serverKeyGuard(bindingTicketsAddToQueue); !blocked {
		t.Fatal(`"a" is not blocked without a server`)
	}
}

func TestRefresh_NoDiskPollWhileConnected(t *testing.T) {
	m := newServerModel(t)
	_, cmd := m.Update(epicsLoadedMsg{})
	if cmd != nil {
		t.Fatalf("a connected load scheduled %v; want no poll timer", cmd)
	}
}

// The shell tells the tab the store changed; a connected tab ignores it, and a
// down tab reloads. The tab keeps no loop of its own.
func TestRefresh_StoreChangedReloadsOnlyWhenDown(t *testing.T) {
	m := newServerModel(t)
	if _, cmd := m.Update(StoreChangedMsg{}); cmd != nil {
		t.Fatal("a connected tab reloaded on a store change")
	}

	down, _ := withLink(m, ServerLinkDown)
	_, cmd := down.Update(StoreChangedMsg{})
	if cmd == nil {
		t.Fatal("a down tab must reload on a store change")
	}
	if _, ok := cmd().(epicsLoadedMsg); !ok {
		t.Fatal("a store change must be a single disk read, not a loop")
	}
}

func TestRefresh_ActivationReloadsOnlyWhenDown(t *testing.T) {
	m := newServerModel(t)
	if m.OnPageActivated() != nil {
		t.Fatal("a connected tab reloaded on activation")
	}
	down, _ := withLink(m, ServerLinkDown)
	if down.OnPageActivated() == nil {
		t.Fatal("a down tab must reload on activation")
	}
}

func TestRefresh_WatchFiresOnStoreChange(t *testing.T) {
	dir := t.TempDir()
	events, stop, err := WatchStore(dir)
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
