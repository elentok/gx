package nativerunner

import "github.com/elentok/gx/harness"

// The frozen status op serialises claude's pid status as it is; the claude
// harness owns reading it.
type PidStatus = harness.PidStatus

const (
	ClaudeStarting = harness.ClaudeStarting
	ClaudeBusy     = harness.ClaudeBusy
	ClaudeIdle     = harness.ClaudeIdle
	ClaudeWaiting  = harness.ClaudeWaiting
)

var ParsePidStatus = harness.ParsePidStatus
