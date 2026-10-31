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

// A herdr run left behind under another config is still probed, but it never
// raises the banner: only agent-runner: herdr does.
func TestHerdr_NonConfiguredHerdrIsProbedWithoutABanner(t *testing.T) {
	t.Parallel()
	h, own, _ := liveUnder(t, newRenamedFake("herdr", nativerunner.VerdictLive), func(c *server.Config) { c.HerdrRetryInterval = 20 * time.Millisecond })
	own.SetHealthErr(errors.New("herdr down"))
	servertest.WaitFor(t, 10*time.Second, "the probe to see herdr down", func() bool {
		log, _ := os.ReadFile(server.LogPath(h.StateDir))
		return strings.Contains(string(log), `"herdr state changed","unavailable":true`)
	})
	if snap, err := h.Client.Snapshot(context.Background()); err != nil || snap.HerdrUnavailable || snap.HerdrText != "" {
		t.Errorf("snapshot banner = %v %q (err %v), want none", snap.HerdrUnavailable, snap.HerdrText, err)
	}
}
