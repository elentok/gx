package server_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/syshealth"
	"github.com/elentok/gx/testutil"
)

const routeGB = 1_000_000_000

// startHealthRoute starts a server with one watched disk and a steerable system source.
func startHealthRoute(t *testing.T, fake *servertest.FakeSystem) *servertest.Harness {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store, repo := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := servertest.WriteProjectRepo(store, "p", repo, ""); err != nil {
		t.Fatal(err)
	}
	disks := &servertest.FakeDisks{}
	disks.PutSized(repo, "d1", "/vol/one", 400*routeGB, 500*routeGB)
	return servertest.StartWithStore(t, store, func(c *server.Config) {
		c.SystemHealth = fake.Read
		c.HealthPollInterval = healthInterval
		c.FreeSpace = disks.Read
		c.GoCacheDir = func() (string, error) { return repo, nil }
		c.DiskPollInterval = 10 * time.Millisecond
	})
}

func healthDisk(r server.HealthReport, id string) *server.HealthDisk {
	for i := range r.Disks.Disks {
		if r.Disks.Disks[i].ID == id {
			return &r.Disks.Disks[i]
		}
	}
	return nil
}

func healthRouteWhen(t *testing.T, h *servertest.Harness, what string, pred func(server.HealthReport) bool) server.HealthReport {
	t.Helper()
	var last server.HealthReport
	ok := testutil.Poll(10*time.Second, 5*time.Millisecond, func() bool {
		var err error
		if last, err = h.Client.Health(context.Background()); err != nil {
			t.Fatal(err)
		}
		return pred(last)
	})
	if !ok {
		t.Fatalf("health = %+v, want %s", last, what)
	}
	return last
}

func TestHealthRoute_HostTimeCPUMemoryAndDisks(t *testing.T) {
	fake := servertest.NewFakeSystem()
	fake.SetBusy(30)
	fake.SetMem(50)
	h := startHealthRoute(t, fake)
	host, _ := os.Hostname()

	rep := healthRouteWhen(t, h, "known CPU and disk d1", func(r server.HealthReport) bool {
		return r.CPU.State == server.HealthStateNormal && healthDisk(r, "d1") != nil
	})
	if rep.Host != host {
		t.Errorf("host = %q, want %q", rep.Host, host)
	}
	if time.Since(rep.Time) > time.Minute || rep.Time.After(time.Now().Add(time.Minute)) {
		t.Errorf("time = %v, want about now", rep.Time)
	}
	if rep.CPU.Percent < 29 || rep.CPU.Percent > 31 || rep.CPU.Thresholds.High != syshealth.CPUHigh || rep.CPU.Thresholds.Critical != syshealth.CPUCritical {
		t.Errorf("cpu = %+v", rep.CPU)
	}
	if rep.Memory.Percent != 50 || rep.Memory.TotalBytes != 16<<30 || rep.Memory.UsedBytes != 8<<30 ||
		rep.Memory.Thresholds.High != syshealth.MemHigh || rep.Memory.Thresholds.Critical != syshealth.MemCritical {
		t.Errorf("memory = %+v", rep.Memory)
	}
	d := healthDisk(rep, "d1")
	if d.FreeBytes != 400*routeGB || d.TotalBytes != 500*routeGB || d.State != server.HealthStateNormal {
		t.Errorf("disk = %+v", d)
	}
}

func TestHealthRoute_PressureOnlyWhenTheSourceSetsIt(t *testing.T) {
	fake := servertest.NewFakeSystem()
	h := startHealthRoute(t, fake)
	rep := healthRouteWhen(t, h, "a memory reading", func(r server.HealthReport) bool { return r.Memory.TotalBytes > 0 })
	if rep.Memory.Pressure != "" {
		t.Errorf("pressure = %q, want none", rep.Memory.Pressure)
	}
	fake.SetPressure(syshealth.PressureWarn)
	healthRouteWhen(t, h, "pressure warn", func(r server.HealthReport) bool { return r.Memory.Pressure == server.PressureWarn })
}

func TestHealthRoute_UnknownCarriesItsReason(t *testing.T) {
	fake := servertest.NewFakeSystem()
	fake.Fail(syshealth.ErrUnavailable)
	h := startHealthRoute(t, fake)
	rep := healthRouteWhen(t, h, "unavailable", func(r server.HealthReport) bool { return r.CPU.Reason == server.HealthReasonUnavailable })
	if rep.Memory.State != server.HealthStateUnknown || rep.Memory.Reason != server.HealthReasonUnavailable {
		t.Errorf("memory = %+v", rep.Memory)
	}
}

// Seam: the Disks report names the disk the D bar follows, the warned one, even
// when another disk has less free space.
func TestHealthRoute_DisksReportNamesTheWorstDisk(t *testing.T) {
	repo, goCache := t.TempDir(), t.TempDir()
	r := startDiskRig(t, map[string]string{"proj": repo}, 20*routeGB, func(c *server.Config) {
		c.GoCacheDir = func() (string, error) { return goCache, nil }
	})
	// d1 is small and normal, d2 is large and under the warn size.
	r.disks.PutSized(repo, "d1", "/vol1", 25*routeGB, 30*routeGB)
	r.disks.PutSized(goCache, "d2", "/vol2", 15*routeGB, 1000*routeGB)
	rep := healthRouteWhen(t, r.h, "d2 as the worst disk", func(rep server.HealthReport) bool { return rep.Disks.WorstID == "d2" })
	if d := healthDisk(rep, "d2"); d.State != server.HealthStateHigh {
		t.Errorf("d2 = %+v, want high", d)
	}
	if rep.Disks.State != server.HealthStateHigh {
		t.Errorf("disk meter = %+v, want high", rep.Disks)
	}
}

func TestHealthRoute_NoDisksMeterIsUnknownWithAReason(t *testing.T) {
	r := startDiskRig(t, nil, 20*routeGB, func(c *server.Config) {
		c.FreeSpace = func(string) (server.DiskReading, error) { return server.DiskReading{}, errors.New("no disk") }
	})
	rep := healthRouteWhen(t, r.h, "an unknown disk meter", func(server.HealthReport) bool { return true })
	if rep.Disks.State != server.HealthStateUnknown || rep.Disks.Reason == server.HealthReasonNone || rep.Disks.WorstID != "" {
		t.Errorf("disks = %+v, want unknown with a reason and no worst disk", rep.Disks)
	}
}
