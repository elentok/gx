// Package syshealth reads the machine's CPU and memory values for the system
// health indicator. A reading never sleeps: CPU is cumulative ticks, and the
// caller takes deltas between two readings.
package syshealth

import (
	"errors"
	"runtime"
)

// ErrUnavailable means this build or OS cannot read the machine's values. Any
// other error means a read that should work failed.
var ErrUnavailable = errors.New("syshealth: system readings unavailable on this build")

// Pressure levels from kern.memorystatus_vm_pressure_level. Zero means the OS
// has no pressure value (Linux).
const (
	PressureNormal   = 1
	PressureWarn     = 2
	PressureCritical = 4
)

// Reading is one snapshot of the machine.
type Reading struct {
	// CPUIdle and CPUTotal are cumulative ticks since boot.
	CPUIdle  uint64
	CPUTotal uint64
	// TickRate is ticks per second.
	TickRate int
	// Wrap32 marks 32-bit counters: take deltas modulo 2^32.
	Wrap32 bool
	Cores  int

	// MemPercent is 100 minus the kernel's available percent.
	MemPercent float64
	MemUsed    uint64
	MemTotal   uint64
	Pressure   int
}

// Reader takes one reading of the machine.
type Reader func() (Reading, error)

// Default returns the reader for the OS gx is running on.
func Default() Reader {
	_, read := selectBackend(runtime.GOOS)
	return read
}

// selectBackend picks the backend for goos. An unknown OS gets a reader that
// reports ErrUnavailable.
func selectBackend(goos string) (string, Reader) {
	switch goos {
	case "linux":
		return "linux", readLinux
	case "darwin":
		return "darwin", readDarwin
	}
	return "none", func() (Reading, error) { return Reading{}, ErrUnavailable }
}
