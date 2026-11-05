package codex

import (
	"os"
	"testing"

	"github.com/elentok/gx/testutil/agentfake"
)

// The test binary doubles as the fake Codex and its holder child, so tests
// drive it as the PTY runner would: a separate process.
func TestMain(m *testing.M) {
	if os.Getenv(agentfake.CodexEnv) == "1" {
		TUICodex()
	}
	if os.Getenv(agentfake.ProgramEnv) == "1" {
		agentfake.Program()
	}
	agentfake.RegisterHarness()
	os.Exit(m.Run())
}
