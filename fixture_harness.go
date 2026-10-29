//go:build gxfixture

package main

import (
	"os"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/testutil/agentfake"
)

// fakeCodexKind is the non-Claude harness a fixture scenario can run live.
const fakeCodexKind agentrunner.Kind = "fakecodex"

// fakeCodex is the agentfake harness with a program that reports idle at
// start, since nothing else sends it a status.
type fakeCodex struct{ agentfake.Harness }

func (h fakeCodex) Launch(opts harness.LaunchOptions) harness.Command {
	return withIdle(h.Harness.Launch(opts))
}

func (h fakeCodex) Resume(id string, opts harness.LaunchOptions) (harness.Command, bool) {
	cmd, ok := h.Harness.Resume(id, opts)
	return withIdle(cmd), ok
}

func withIdle(cmd harness.Command) harness.Command {
	cmd.Env = append(cmd.Env, agentfake.ProgramIdleEnv+"=1")
	return cmd
}

func init() {
	harness.Register(fakeCodexKind, fakeCodex{})
	// The holder is this binary too, and its env may carry the marker.
	if len(os.Args) > 1 && os.Args[1] != "agent-holder" && os.Getenv(agentfake.ProgramEnv) == "1" {
		agentfake.Program()
	}
}
