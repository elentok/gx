package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/cpuwatch"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// cpuSpikeServer starts a server whose store has a gx project, with opts on top
// of a cpu-watchdog that is on.
func cpuSpikeServer(t *testing.T, opts ...func(*server.Config)) (*servertest.Harness, string) {
	t.Helper()
	store := t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, server.OutboxProject), 0o755); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, server.OutboxProject, testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.CPUWatchdog = config.DefaultCPUWatchdogConfig()
		for _, o := range opts {
			o(c)
		}
	})
	return h, store
}

var testSpike = cpuwatch.Spike{Source: "tui", PID: 42, Dir: "/evidence", Samples: []cpuwatch.Sample{
	{At: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC), Self: 18, Children: 4},
	{At: time.Date(2026, 10, 18, 12, 0, 10, 0, time.UTC), Self: 20, Children: 6},
}}

func TestCPUSpike_MaintainerFilesOneTicketThenCoolsDown(t *testing.T) {
	h, store := cpuSpikeServer(t, withEvolve(config.EvolveMaintainer))
	ctx := context.Background()
	res, err := h.Client.CPUSpike(ctx, testSpike)
	if err != nil || res.Address == "" {
		t.Fatalf("CPUSpike = %+v, %v; want a filed ticket", res, err)
	}
	epics, _ := filepath.Glob(filepath.Join(store, server.OutboxProject, "cpu-spike-tui-*"))
	if len(epics) != 1 {
		t.Fatalf("cpu-spike epics = %v, want one", epics)
	}
	issues, _ := filepath.Glob(filepath.Join(epics[0], "issues", "*.md"))
	if len(issues) != 1 {
		t.Fatalf("issues = %v, want one", issues)
	}
	body, _ := os.ReadFile(issues[0])
	for _, want := range []string{"gx tui (pid 42)", "24% of a core", "/evidence/cpu.pprof", "20s"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("ticket lacks %q:\n%s", want, body)
		}
	}

	again, err := h.Client.CPUSpike(ctx, testSpike)
	if err != nil || again.Address != "" || !strings.Contains(again.Message, "cooling down") {
		t.Fatalf("second CPUSpike = %+v, %v; want a cooldown refusal", again, err)
	}
}

func TestCPUSpike_OnlyLoggedOutsideMaintainerMode(t *testing.T) {
	for _, mode := range []string{config.EvolveOff, config.EvolveContributor} {
		t.Run(mode, func(t *testing.T) {
			h, store := cpuSpikeServer(t, withEvolve(mode))
			res, err := h.Client.CPUSpike(context.Background(), testSpike)
			if err != nil || res.Address != "" || res.Message == "" {
				t.Fatalf("CPUSpike = %+v, %v; want no ticket and a reason", res, err)
			}
			if epics, _ := filepath.Glob(filepath.Join(store, server.OutboxProject, "cpu-spike-*")); len(epics) != 0 {
				t.Fatalf("cpu-spike epics = %v, want none", epics)
			}
		})
	}
}

func TestCPUSpike_OffFilesNothing(t *testing.T) {
	h, _ := cpuSpikeServer(t, withEvolve(config.EvolveMaintainer), func(c *server.Config) { c.CPUWatchdog.Enabled = false })
	res, err := h.Client.CPUSpike(context.Background(), testSpike)
	if err != nil || res.Address != "" || !strings.Contains(res.Message, "off") {
		t.Fatalf("CPUSpike = %+v, %v; want it refused as off", res, err)
	}
}
