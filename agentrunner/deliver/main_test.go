package deliver_test

import (
	"os"
	"testing"

	"github.com/elentok/gx/testutil/agentfake"
)

// The test binary doubles as the fake program the held-program tests launch;
// without this hook it would fork without end.
func TestMain(m *testing.M) {
	if os.Getenv(agentfake.ProgramEnv) == "1" {
		agentfake.Program()
	}
	agentfake.RegisterHarness()
	os.Exit(m.Run())
}
