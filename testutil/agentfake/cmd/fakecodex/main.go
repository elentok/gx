// Command fakecodex is the fake Codex binary e2e fixtures install as "codex"
// on PATH. It runs the interactive Codex fake on the shared agentfake core.
package main

import "github.com/elentok/gx/testutil/agentfake"

func main() {
	agentfake.TUICodex()
}
