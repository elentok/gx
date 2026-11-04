package disk_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/gocachegate"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestDiskCleanGate_CleanNowWaitsForARegisteredBuild(t *testing.T) {
	t.Parallel()
	r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb)
	r.known(t)
	release := gocachegate.Register(r.h.StateDir)
	done := r.cleanNowAsync()
	r.letWatcherRead(t)
	if got := r.clean.recorded(); len(got) != 0 {
		t.Fatalf("cleanup ran while a build held the gate: %v", got)
	}
	release()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("clean now did not finish after the build released the gate")
	}
	if got := r.clean.recorded(); len(got) != 1 {
		t.Errorf("calls = %v, want 1", got)
	}
}

func TestDiskCleanGate_AutomaticCleanAndCustomCommandWait(t *testing.T) {
	t.Parallel()
	r := startCleanRig(t, map[string]string{"custom": overrideCmd}, 0, 10*gb)
	release := gocachegate.Register(r.h.StateDir)
	r.setFree(3 * gb)
	r.letWatcherRead(t)
	if got := r.clean.recorded(); len(got) != 0 {
		t.Fatalf("custom cleanup ran while a build held the gate: %v", got)
	}
	release()
	got := r.waitCalls(t, 1)
	if want := "make tidy @ " + r.repos["custom"]; got[0] != want {
		t.Errorf("calls = %v, want %q", got, want)
	}
}

func TestDiskCleanGate_CleansAfterTheCapAndLogsIt(t *testing.T) {
	t.Parallel()
	r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb, func(c *server.Config) {
		c.CacheGateCap = 100 * time.Millisecond
	})
	release := gocachegate.Register(r.h.StateDir)
	defer release()
	r.setFree(3 * gb)
	r.waitCalls(t, 1)
	servertest.Await(t, func() bool {
		log, _ := os.ReadFile(server.LogPath(r.h.StateDir))
		return strings.Contains(string(log), "went ahead after the build wait cap")
	})
}
