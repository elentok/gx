package serverapi

import (
	"github.com/elentok/gx/server/wire"
)

// notRunning is the single refusal a write gets when no server answers.
func notRunning(target string) wire.TargetResult {
	return wire.RefusedTarget(target, wire.ReasonServerNotRunning, "no server is running; start it with `gx server start`")
}
