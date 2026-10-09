package runnerfake_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/runnertest"
	"github.com/elentok/gx/testutil/runnerfake"
)

func TestConformance(t *testing.T) {
	runnertest.Run(t, func(t *testing.T) runnertest.Harness {
		r := runnerfake.NewRunner()
		return runnertest.Harness{
			Runner: r,
			FinishTurn: func(t *testing.T, s agentrunner.Session) {
				r.SetState(s.Label, agentrunner.StateIdle, "")
			},
			Block: func(t *testing.T, s agentrunner.Session, reason string) {
				r.SetState(s.Label, agentrunner.StateBlocked, reason)
			},
			RateLimit: func(t *testing.T, s agentrunner.Session, resetAt time.Time) {
				r.SetRateLimit(s.Label, resetAt)
			},
			// The fake's sessions live in memory, so a restarted gx sees the
			// same runner.
			Restart: func(t *testing.T) agentrunner.Runner { return r },
		}
	})
}
