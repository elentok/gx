package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestServerHealth_TextNamesHostCPUMemoryAndEachDisk(t *testing.T) {
	h := startDiskStatusServer(t, config.DiskSpaceConfig{WarnBelow: 20 * diskGB, CleanBelow: 10 * diskGB})
	host, _ := os.Hostname()
	waitHealthKnown(t, h)

	var out bytes.Buffer
	if err := runServerHealth(context.Background(), h.Client, &out, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"host " + host, "cpu:", "memory:", "disk /vol/low:", "disk /vol/ok:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestServerHealth_JSONMatchesTheRoute(t *testing.T) {
	h := startDiskStatusServer(t, config.DiskSpaceConfig{WarnBelow: 20 * diskGB, CleanBelow: 10 * diskGB})
	waitHealthKnown(t, h)

	var out bytes.Buffer
	if err := runServerHealth(context.Background(), h.Client, &out, true); err != nil {
		t.Fatal(err)
	}
	var got server.HealthReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", out.String(), err)
	}
	host, _ := os.Hostname()
	if got.Host != host || len(got.Disks.Disks) < 2 || got.Disks.Disks[0].TotalBytes == 0 || got.Disks.WarnBelow != "20G" {
		t.Errorf("--json = %s", out.String())
	}
}

// waitHealthKnown waits until the watcher has taken a first system reading.
func waitHealthKnown(t *testing.T, h *servertest.Harness) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		rep, err := h.Client.Health(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if rep.Memory.State != server.HealthStateUnknown {
			return
		}
	}
	t.Fatal("watcher never read the system")
}
