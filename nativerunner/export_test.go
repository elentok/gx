package nativerunner

import "testing"

// PS is the real process table.
var PS ProcessTable = psTable{}

// SetHolderOptionalOps makes holders started by the test advertise ops.
func SetHolderOptionalOps(t *testing.T, ops []string) {
	old := holderOptionalOps
	holderOptionalOps = ops
	t.Cleanup(func() { holderOptionalOps = old })
}
