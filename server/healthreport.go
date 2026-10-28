package server

import (
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
type DisksReport struct {
	WarnBelow  string       `json:"warn_below"`
	CleanBelow string       `json:"clean_below"`
	Disks      []HealthDisk `json:"disks"`
}

// HealthReport is the health route's result. It holds the watcher's stored
// values and takes no new reading.
type HealthReport struct {
	Host   string       `json:"host"`
	Time   time.Time    `json:"time"`
	CPU    CPUReport    `json:"cpu"`
	Memory MemoryReport `json:"memory"`
	Disks  DisksReport  `json:"disks"`
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

func (s *Server) healthReport() HealthReport {
	w := &s.healthW
	w.mu.Lock()
	h, l := w.health, w.latest
	w.mu.Unlock()

	ds := s.settings.Load().DiskSpace
	cleanBelow := uint64(ds.CleanBelow)
	disks := make([]HealthDisk, 0)
	for _, d := range s.diskStatuses(false) {
		state := syshealth.Disk{Free: d.FreeBytes, Total: d.TotalBytes, Warned: d.Warned, Held: d.Held, CleanBelow: cleanBelow}.State()
		disks = append(disks, HealthDisk{ID: d.ID, Name: d.Name, FreeBytes: d.FreeBytes, TotalBytes: d.TotalBytes,
			State: HealthState(state), Held: d.Held, Projects: d.Projects, ReadAt: d.ReadAt})
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
		Disks: DisksReport{WarnBelow: sizeOrOff(ds.WarnBelow), CleanBelow: sizeOrOff(ds.CleanBelow), Disks: disks},
	}
}

func (s *Server) healthRead(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.healthReport())
}
