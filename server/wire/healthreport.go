package wire

import (
	"time"
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

// HealthState is what a Tray meter shows.
type HealthState string

const (
	HealthStateNormal   HealthState = "normal"
	HealthStateHigh     HealthState = "high"
	HealthStateCritical HealthState = "critical"
	HealthStateUnknown  HealthState = "unknown"
)

// HealthReason says why a meter is HealthStateUnknown; it is empty otherwise.
type HealthReason string

const (
	HealthReasonNone        HealthReason = ""
	HealthReasonWaiting     HealthReason = "waiting"
	HealthReasonUnavailable HealthReason = "unavailable"
	HealthReasonError       HealthReason = "error"
)

// MeterHealth is one Tray meter: its fill (0 to 4), state and reading time.
type MeterHealth struct {
	Level  int          `json:"level"`
	State  HealthState  `json:"state"`
	Reason HealthReason `json:"reason,omitempty"`
	// ReadAt is when the meter's value was last read; zero before the first read.
	ReadAt time.Time `json:"read_at"`
}

// Health is the snapshot's system health: CPU, memory and disk meters.
type Health struct {
	CPU    MeterHealth `json:"cpu"`
	Memory MeterHealth `json:"memory"`
	Disk   MeterHealth `json:"disk"`
}

// EventHealthChanged is emitted when a health meter's level or state changes.
const EventHealthChanged = "health-changed"
