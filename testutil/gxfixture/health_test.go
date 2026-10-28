package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

// setHealth replaces the system control file through a rename, as a flow does.
func (r *running) setHealth(t *testing.T, value string) {
	t.Helper()
	tmp := filepath.Join(r.HealthDir, "system.tmp")
	if err := os.WriteFile(tmp, []byte(value+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(r.HealthDir, "system")); err != nil {
		t.Fatal(err)
	}
}

// waitHealth polls `gx server health --json` until ok holds.
func (r *running) waitHealth(t *testing.T, what string, ok func(server.HealthReport) bool) server.HealthReport {
	t.Helper()
	var rep server.HealthReport
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := json.Unmarshal([]byte(r.gx(t, "server", "health", "--json")), &rep); err != nil {
			t.Fatal(err)
		}
		if ok(rep) {
			return rep
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("health never showed %s: %+v", what, rep)
	return rep
}

func TestHealth_TheControlFilesSteerTheMeters(t *testing.T) {
	r := start(t, "health")
	defer r.stop(t)

	r.setHealth(t, "cpu=90 mem=85 pressure=warn")
	rep := r.waitHealth(t, "busy values", func(h server.HealthReport) bool {
		return h.CPU.State != server.HealthStateUnknown && h.CPU.Average > 80 && h.Memory.Average > 80
	})
	if rep.Memory.Pressure != server.PressureWarn {
		t.Errorf("pressure = %q, want %q", rep.Memory.Pressure, server.PressureWarn)
	}

	r.setHealth(t, "cpu=unavailable mem=85")
	rep = r.waitHealth(t, "unavailable CPU", func(h server.HealthReport) bool {
		return h.CPU.State == server.HealthStateUnknown && h.CPU.Reason == server.HealthReasonUnavailable
	})

	r.setDisk(t, "free", "8G/500G")
	r.waitHealth(t, "8G free of 500G", func(h server.HealthReport) bool {
		return len(h.Disks.Disks) == 1 && h.Disks.Disks[0].FreeBytes == 8_000_000_000 && h.Disks.Disks[0].TotalBytes == 500_000_000_000
	})
}

// cpuRow is the first CPU cause row of kind, or false.
func cpuRow(h server.HealthReport, kind string) (server.CauseRow, bool) {
	for _, row := range h.Causes.CPU.Rows {
		if row.Kind == kind {
			return row, true
		}
	}
	return server.CauseRow{}, false
}

// waitCauses polls with wait=true until the CPU list is out of its reading marker and ok holds.
func (r *running) waitCauses(t *testing.T, what string, ok func(server.HealthReport) bool) server.HealthReport {
	t.Helper()
	return r.waitHealth(t, what, func(h server.HealthReport) bool {
		return h.Causes.CPU.Marker == server.CauseMarkerNone && ok(h)
	})
}

func TestHealth_TheProcessTableNamesTheAgentByItsTicket(t *testing.T) {
	r := start(t, "health")
	defer r.stop(t)

	const addr = project + ":health-epic/01"
	rep := r.waitCauses(t, "agent, process and other-users rows", func(h server.HealthReport) bool {
		a, ok := cpuRow(h, "agent")
		_, plain := cpuRow(h, "process")
		_, other := cpuRow(h, "other-users")
		return ok && plain && other && a.Ticket == addr
	})
	if a, _ := cpuRow(rep, "agent"); a.Busiest != "go test" {
		t.Errorf("busiest = %q, want go test", a.Busiest)
	}
	if p, _ := cpuRow(rep, "process"); p.Label != "build.js" {
		t.Errorf("process label = %q, want build.js", p.Label)
	}

	processes := filepath.Join(r.HealthDir, "processes")
	b, err := os.ReadFile(processes)
	if err != nil {
		t.Fatal(err)
	}
	led := strings.Replace(string(b), " 800 100M node", " 6000 100M node", 1)
	if led == string(b) {
		t.Fatalf("no node row to change in %q", b)
	}
	led = strings.Replace(led, "busy 4500", "busy 8000", 1)
	if err := writeHealthFile(processes, led); err != nil {
		t.Fatal(err)
	}
	r.waitCauses(t, "build.js leading", func(h server.HealthReport) bool {
		rows := h.Causes.CPU.Rows
		return len(rows) > 0 && rows[0].Kind == "process" && rows[0].Label == "build.js"
	})
}

func TestHealth_EveryOtherScenarioStartsCalm(t *testing.T) {
	r := start(t, "empty")
	defer r.stop(t)

	got, err := os.ReadFile(filepath.Join(r.HealthDir, "system"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != calmHealth+"\n" {
		t.Errorf("system = %q, want %q", got, calmHealth)
	}
	r.waitHealth(t, "the calm values", func(h server.HealthReport) bool {
		return h.CPU.State != server.HealthStateUnknown && math.Abs(h.CPU.Average-20) < 0.01 && math.Abs(h.Memory.Average-50) < 0.01
	})
}
