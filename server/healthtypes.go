package server

import "time"

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
