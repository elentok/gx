package server_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestStart_LogsRunsStillOnANonConfiguredRunner(t *testing.T) {
	t.Parallel()
	h, _, _ := liveUnderOther(t)
	var log []byte
	servertest.WaitFor(t, 10*time.Second, "the flush line for runner a", func() bool {
		log, _ = os.ReadFile(server.LogPath(h.StateDir))
		return strings.Contains(string(log), "1 runs still on a")
	})
	if strings.Contains(string(log), "runs still on b") {
		t.Errorf("log names the configured runner:\n%s", log)
	}
}

func TestHerdr_RunOnANonConfiguredHerdrIsNotProbed(t *testing.T) {
	t.Parallel()
	h, own, _ := liveUnder(t, newRenamedFake("herdr", nativerunner.VerdictLive), func(c *server.Config) { c.HerdrRetryInterval = 20 * time.Millisecond })
	own.SetHealthErr(errors.New("herdr down"))
	time.Sleep(100 * time.Millisecond) // negative wait: a probe would show within a few 20ms retries
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.HerdrUnavailable {
		t.Error("a live run on a non-configured herdr made the server probe it")
	}
}
