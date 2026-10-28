package server_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// The server's config env reaches the store's git children and, through the
// harness default, keeps auto maintenance off.
func TestStoreCommits_GitChildrenGetTheConfigEnvWithoutAutoMaintenance(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	trace := filepath.Join(t.TempDir(), "trace2.json")
	servertest.WriteTicket(t, store, "proj", "epic", "01", "first", "")
	servertest.StartWithStore(t, store, servertest.WithEnv("GIT_TRACE2_EVENT="+trace), func(c *server.Config) {
		c.StoreCommitDebounce = time.Hour
	})
	servertest.Poll(5*time.Second, func() bool { return storeLog(t, store) != "" })
	raw, err := os.ReadFile(trace)
	if err != nil || len(raw) == 0 {
		t.Fatalf("trace file %q, %v: the config env did not reach the store's git", raw, err)
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		var ev struct {
			Event string   `json:"event"`
			Argv  []string `json:"argv"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Event != "child_start" {
			continue
		}
		if slices.Contains(ev.Argv, "maintenance") || slices.Contains(ev.Argv, "gc") {
			t.Errorf("a store commit started auto maintenance: %v", ev.Argv)
		}
	}
}

func TestStoreCommits_ServerCommitsEditsMadeWhileDownOnStart(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic", "01", "first", "")
	servertest.StartWithStore(t, store, func(c *server.Config) {
		c.StoreCommitDebounce = time.Hour
	})
	servertest.Poll(5*time.Second, func() bool { return storeLog(t, store) != "" })
	if got := storeLog(t, store); !strings.HasPrefix(got, "sync: ") {
		t.Errorf("log = %q, want a sync commit of the down-time edit", got)
	}
}
