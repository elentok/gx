package health

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/elentok/gx/server/disk"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/syshealth"
)

func pressureName(level int) string {
	switch level {
	case syshealth.PressureNormal:
		return wire.PressureNormal
	case syshealth.PressureWarn:
		return wire.PressureWarn
	case syshealth.PressureCritical:
		return wire.PressureCritical
	}
	return ""
}

func (s *Health) healthReport(ctx context.Context, wait bool) wire.HealthReport {
	w := &s.healthW
	w.mu.Lock()
	h, l := w.health, w.latest
	w.mu.Unlock()

	ds := s.settings.Load().DiskSpace
	cleanBelow := uint64(ds.CleanBelow)
	statuses := s.disks.Statuses(false)
	in := syshealthDisks(statuses, cleanBelow)
	disks := make([]wire.HealthDisk, 0, len(statuses))
	for i, d := range statuses {
		disks = append(disks, wire.HealthDisk{ID: d.ID, Name: d.Name, FreeBytes: d.FreeBytes, TotalBytes: d.TotalBytes,
			State: wire.HealthState(in[i].State()), Held: d.Held, Projects: d.Projects, ReadAt: d.ReadAt})
	}
	worstID := ""
	if worst := syshealth.WorstDisk(in); worst >= 0 {
		worstID = statuses[worst].ID
	}
	host, _ := os.Hostname()
	return wire.HealthReport{
		Host: host,
		Time: time.Now(),
		CPU: wire.CPUReport{Percent: l.cpuPct, Average: l.cpuAvg, Level: h.CPU.Level, State: h.CPU.State, Reason: h.CPU.Reason,
			Thresholds: wire.HealthThresholds{High: syshealth.CPUHigh, Critical: syshealth.CPUCritical}, ReadAt: h.CPU.ReadAt},
		Memory: wire.MemoryReport{Percent: l.memPct, UsedBytes: l.memUsed, TotalBytes: l.memTotal, Average: l.memAvg,
			Pressure: pressureName(l.pressure), Level: h.Memory.Level, State: h.Memory.State, Reason: h.Memory.Reason,
			Thresholds: wire.HealthThresholds{High: syshealth.MemHigh, Critical: syshealth.MemCritical}, ReadAt: h.Memory.ReadAt},
		Disks: wire.DisksReport{WarnBelow: disk.SizeOrOff(ds.WarnBelow), CleanBelow: disk.SizeOrOff(ds.CleanBelow),
			State: h.Disk.State, Level: h.Disk.Level, Reason: h.Disk.Reason, WorstID: worstID, Disks: disks},
		Causes: s.healthCauses(ctx, wait),
	}
}

func (s *Health) healthRead(w http.ResponseWriter, r *http.Request) {
	_, wait := r.URL.Query()["wait"]
	wire.WriteJSON(w, s.healthReport(r.Context(), wait))
}
