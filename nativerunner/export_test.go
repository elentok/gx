package nativerunner

import (
	"testing"
	"time"
)

// PS is the real process table.
var PS ProcessTable = psTable{}

// SetHolderOptionalOps makes holders started by the test advertise ops.
func SetHolderOptionalOps(t *testing.T, ops []string) {
	old := holderOptionalOps
	holderOptionalOps = ops
	t.Cleanup(func() { holderOptionalOps = old })
}

// HolderOptionalOps and HolderDenyTimeout configure a holder the test binary
// runs as a subprocess, which has no *testing.T to restore them.
var (
	HolderOptionalOps = &holderOptionalOps
	HolderDenyTimeout = &holderDenyTimeout
)

// SetHolderDenyTimeout shortens deny-reason's waits for the test.
func SetHolderDenyTimeout(t *testing.T, d time.Duration) {
	old := holderDenyTimeout
	holderDenyTimeout = d
	t.Cleanup(func() { holderDenyTimeout = old })
}
