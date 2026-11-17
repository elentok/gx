package sched_test

import (
	"os"
	"strings"
	"testing"
	"time"

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
