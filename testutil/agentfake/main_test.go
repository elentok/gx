package agentfake

import (
	"os"
	"testing"
)

// The test binary doubles as the fake program the harness tests launch.
func TestMain(m *testing.M) {
	if os.Getenv(ProgramEnv) == "1" {
		Program()
	}
	RegisterHarness()
	os.Exit(m.Run())
}
