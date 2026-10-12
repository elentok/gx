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
)

func TestStart_LogsRunsStillOnANonConfiguredRunner(t *testing.T) {
	h, _, _ := liveUnderOther(t)
	deadline := time.Now().Add(10 * time.Second)
	for {
		log, _ := os.ReadFile(server.LogPath(h.StateDir))
		if strings.Contains(string(log), "1 runs still on a") {
			if strings.Contains(string(log), "runs still on b") {
				t.Errorf("log names the configured runner:\n%s", log)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no drain line for runner a:\n%s", log)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestHerdr_RunOnANonConfiguredHerdrIsNotProbed(t *testing.T) {
	h, own, _ := liveUnder(t, newRenamedFake("herdr", nativerunner.VerdictLive), func(c *server.Config) { c.HerdrRetryInterval = 20 * time.Millisecond })
	own.SetHealthErr(errors.New("herdr down"))
	time.Sleep(100 * time.Millisecond)
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.HerdrUnavailable {
		t.Error("a live run on a non-configured herdr made the server probe it")
	}
}
