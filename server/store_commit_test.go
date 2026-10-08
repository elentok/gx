package server_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func storeLog(t *testing.T, store string) string {
	t.Helper()
	out, _ := exec.Command("git", "-C", store, "log", "--format=%s").Output()
	return strings.TrimSpace(string(out))
}

func TestStoreCommits_ServerCommitsEditsMadeWhileDownOnStart(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic", "01", "first", "")
	servertest.StartWithStore(t, store, func(c *server.Config) {
		c.StoreCommitDebounce = time.Hour
	})
	deadline := time.Now().Add(5 * time.Second)
	for storeLog(t, store) == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := storeLog(t, store); !strings.HasPrefix(got, "sync: ") {
		t.Errorf("log = %q, want a sync commit of the down-time edit", got)
	}
}

func TestStoreCommits_InProcessSchedulerLeavesCommitsToItsOwnLoop(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic", "01", "first", "")
	servertest.StartWithStore(t, store, func(c *server.Config) { c.StoreCommitDebounce = time.Hour })
	if _, err := os.Stat(store + "/.git"); err == nil {
		t.Errorf("server touched the store's git repo while not selected")
	}
}
