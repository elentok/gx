package herdrrunner_test

import (
	"os"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/agentrunner/runnertest"
	"github.com/elentok/gx/testutil/herdrfake"
)

func TestMain(m *testing.M) {
	herdrfake.RunHelperProcess()
	os.Exit(m.Run())
}

func TestConformance(t *testing.T) {
	runnertest.Run(t, func(t *testing.T) runnertest.Harness {
		state := herdrfake.NewState(t)
		herdrfake.StartAgentHost(t, state)
		setStatus := func(t *testing.T, s agentrunner.Session, status, rule string) {
			if err := state.SetAgentStatus(s.Label, status, rule); err != nil {
				t.Fatal(err)
			}
		}
		return runnertest.Harness{
			Runner: herdrrunner.New(),
			FinishTurn: func(t *testing.T, s agentrunner.Session) {
				setStatus(t, s, "idle", "")
			},
			Block: func(t *testing.T, s agentrunner.Session, reason string) {
				setStatus(t, s, "blocked", reason)
			},
			RateLimit: func(t *testing.T, s agentrunner.Session, resetAt time.Time) {
				t.Skip("herdr RateLimit lands in a sibling ticket")
			},
			Restart: func(t *testing.T) agentrunner.Runner { return herdrrunner.New() },
		}
	})
}
