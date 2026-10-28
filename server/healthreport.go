package server

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/elentok/gx/syshealth"
)

// Pressure names the macOS memory pressure level; the field is absent on Linux.
const (
	PressureNormal   = "normal"
	PressureWarn     = "warn"
	PressureCritical = "critical"
)

// HealthThresholds are the average percents where a meter turns high and critical.
type HealthThresholds struct {
	High     float64 `json:"high"`
	Critical float64 `json:"critical"`
}

// CPUReport is the CPU meter in the health route.
type CPUReport struct {
	Percent    float64          `json:"percent"`
	Average    float64          `json:"average"`
	Level      int              `json:"level"`
	State      HealthState      `json:"state"`
	Reason     HealthReason     `json:"reason,omitempty"`
	Thresholds HealthThresholds `json:"thresholds"`
	ReadAt     time.Time        `json:"read_at"`
}

// MemoryReport is the memory meter in the health route.
type MemoryReport struct {
	Percent    float64          `json:"percent"`
	UsedBytes  uint64           `json:"used_bytes"`
	TotalBytes uint64           `json:"total_bytes"`
	Average    float64          `json:"average"`
	Pressure   string           `json:"pressure,omitempty"`
	Level      int              `json:"level"`
	State      HealthState      `json:"state"`
	Reason     HealthReason     `json:"reason,omitempty"`
	Thresholds HealthThresholds `json:"thresholds"`
	ReadAt     time.Time        `json:"read_at"`
}

// HealthDisk is one watched disk in the health route.
type HealthDisk struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	FreeBytes  uint64      `json:"free_bytes"`
	TotalBytes uint64      `json:"total_bytes"`
	State      HealthState `json:"state"`
	Held       bool        `json:"held"`
	Projects   []string    `json:"projects"`
	ReadAt     time.Time   `json:"read_at"`
}

// DisksReport is every watched disk with the disk thresholds as size strings.
// State, Level and Reason are the disk meter the Tray's D bar shows; WorstID
// is the disk that meter follows, empty when there are no disks.
type DisksReport struct {
	WarnBelow  string       `json:"warn_below"`
	CleanBelow string       `json:"clean_below"`
	State      HealthState  `json:"state"`
	Level      int          `json:"level"`
	Reason     HealthReason `json:"reason,omitempty"`
	WorstID    string       `json:"worst_id,omitempty"`
	Disks      []HealthDisk `json:"disks"`
}

// CauseMarker replaces a cause list's rows: the process table is unavailable,
// or the CPU list is waiting for its second read.
type CauseMarker string

const (
	CauseMarkerNone        CauseMarker = ""
	CauseMarkerReading     CauseMarker = "reading"
	CauseMarkerUnavailable CauseMarker = "unavailable"
)

// CauseRow is one row of a cause list. CPU is a percent of the whole machine.
type CauseRow struct {
	Kind     string  `json:"kind"`
	Label    string  `json:"label"`
	Busiest  string  `json:"busiest,omitempty"`
	Ticket   string  `json:"ticket,omitempty"`
	CPU      float64 `json:"cpu"`
	MemBytes uint64  `json:"mem_bytes"`
}

// CauseList is the top rows of one list, or a marker in their place.
type CauseList struct {
	Marker CauseMarker `json:"marker,omitempty"`
	Rows   []CauseRow  `json:"rows"`
}

// HealthCauses are the top 3 by CPU and by memory.
type HealthCauses struct {
	CPU    CauseList `json:"cpu"`
	Memory CauseList `json:"memory"`
}

// HealthReport is the health route's result. The meters are the watcher's
// stored values; only the cause lists can cause a new process read.
type HealthReport struct {
	Host   string       `json:"host"`
	Time   time.Time    `json:"time"`
	CPU    CPUReport    `json:"cpu"`
	Memory MemoryReport `json:"memory"`
	Disks  DisksReport  `json:"disks"`
	Causes HealthCauses `json:"causes"`
}

func pressureName(level int) string {
	switch level {
	case syshealth.PressureNormal:
		return PressureNormal
	case syshealth.PressureWarn:
		return PressureWarn
	case syshealth.PressureCritical:
		return PressureCritical
	}
	return ""
}

func (s *Server) healthReport(ctx context.Context, wait bool) HealthReport {
	w := &s.healthW
	w.mu.Lock()
	h, l := w.health, w.latest
	w.mu.Unlock()

	ds := s.settings.Load().DiskSpace
	cleanBelow := uint64(ds.CleanBelow)
	statuses := s.diskStatuses(false)
	in := syshealthDisks(statuses, cleanBelow)
	disks := make([]HealthDisk, 0, len(statuses))
	for i, d := range statuses {
		disks = append(disks, HealthDisk{ID: d.ID, Name: d.Name, FreeBytes: d.FreeBytes, TotalBytes: d.TotalBytes,
			State: HealthState(in[i].State()), Held: d.Held, Projects: d.Projects, ReadAt: d.ReadAt})
	}
	worstID := ""
	if worst := syshealth.WorstDisk(in); worst >= 0 {
		worstID = statuses[worst].ID
	}
	host, _ := os.Hostname()
	return HealthReport{
		Host: host,
		Time: time.Now(),
		CPU: CPUReport{Percent: l.cpuPct, Average: l.cpuAvg, Level: h.CPU.Level, State: h.CPU.State, Reason: h.CPU.Reason,
			Thresholds: HealthThresholds{High: syshealth.CPUHigh, Critical: syshealth.CPUCritical}, ReadAt: h.CPU.ReadAt},
		Memory: MemoryReport{Percent: l.memPct, UsedBytes: l.memUsed, TotalBytes: l.memTotal, Average: l.memAvg,
			Pressure: pressureName(l.pressure), Level: h.Memory.Level, State: h.Memory.State, Reason: h.Memory.Reason,
			Thresholds: HealthThresholds{High: syshealth.MemHigh, Critical: syshealth.MemCritical}, ReadAt: h.Memory.ReadAt},
		Disks: DisksReport{WarnBelow: sizeOrOff(ds.WarnBelow), CleanBelow: sizeOrOff(ds.CleanBelow),
			State: h.Disk.State, Level: h.Disk.Level, Reason: h.Disk.Reason, WorstID: worstID, Disks: disks},
		Causes: s.healthCauses(ctx, wait),
	}
}

func (s *Server) healthRead(w http.ResponseWriter, r *http.Request) {
	_, wait := r.URL.Query()["wait"]
	writeJSON(w, s.healthReport(r.Context(), wait))
}
