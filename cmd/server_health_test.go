package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/syshealth"
	"github.com/elentok/gx/testutil"
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

// Seam: the verb calls the route with `wait`, so a quiet server answers with the
// full CPU list, not the reading marker (story 55).
func TestServerHealth_UsesWaitAndPrintsBothLists(t *testing.T) {
	h := startDiskStatusServer(t, config.DiskSpaceConfig{WarnBelow: 20 * diskGB, CleanBelow: 10 * diskGB})
	h.Processes.Set(
		syshealth.Process{PID: 4001, PPID: 1, StartUnix: 1000, Comm: "busybee", CPUTicks: 400, MemBytes: 1 << 20},
		syshealth.Process{PID: 4002, PPID: 1, StartUnix: 1000, Comm: "hog", CPUTicks: 10, MemBytes: 1 << 30},
	)

	var out bytes.Buffer
	if err := runServerHealth(context.Background(), h.Client, &out, false); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	cpu, mem, _ := strings.Cut(text, "top by memory:")
	if strings.Contains(text, "reading") || !strings.Contains(cpu, "busybee 40.0%") || !strings.Contains(mem, "hog 1073741824") {
		t.Errorf("output lacks the lists:\n%s", text)
	}
}

// Seam 3: an agent row prints as `<address> · <busiest process>`.
func TestServerHealth_AgentRowShowsAddressAndBusiestProcess(t *testing.T) {
	list := server.CauseList{Rows: []server.CauseRow{
		{Kind: "agent", Label: "gx:foo/03", Ticket: "gx:foo/03", Busiest: "go test", CPU: 12.5},
		{Kind: "process", Label: "hog", CPU: 3},
	}}
	got := causeLines("top by cpu", list, func(r server.CauseRow) string { return fmt.Sprintf("%.1f%%", r.CPU) })
	want := []string{"top by cpu:", "  gx:foo/03 · go test 12.5%", "  hog 3.0%"}
	if !slices.Equal(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
}

// waitHealthKnown waits until the watcher has taken a first system reading.
func waitHealthKnown(t *testing.T, h *servertest.Harness) {
	t.Helper()
	known := testutil.Poll(10*time.Second, 10*time.Millisecond, func() bool {
		rep, err := h.Client.Health(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return rep.Memory.State != server.HealthStateUnknown
	})
	if !known {
		t.Fatal("watcher never read the system")
	}
}
