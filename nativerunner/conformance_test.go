package nativerunner_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/runnertest"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestHeadless_Conformance(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runnertest.Run(t, func(t *testing.T) runnertest.Harness {
		root := t.TempDir()
		t.Setenv(agentfake.ClaudeEnv, "1")
		// Claude runs in its own session, so it outlives the test unless
		// killed; Stop isn't enough once a Restart left two runners on it.
		t.Cleanup(func() { killAgents(root) })
		newRunner := func() *nativerunner.Headless {
			return &nativerunner.Headless{
				Root:           root,
				Claude:         exe,
				PollInterval:   5 * time.Millisecond,
				StopGrace:      2 * time.Second,
				InterruptGrace: 2 * time.Second,
				PromptTimeout:  500 * time.Millisecond,
			}
		}
		control := func(t *testing.T, s agentrunner.Session, c agentfake.Control) {
			t.Helper()
			if err := agentfake.SendControl(filepath.Join(root, s.Label, nativerunner.StdinFile), c); err != nil {
				t.Fatalf("control %s: %v", c.Action, err)
			}
		}
		runner := newRunner()
		return runnertest.Harness{
			Runner: runner,
			FinishTurn: func(t *testing.T, s agentrunner.Session) {
				control(t, s, agentfake.Control{Action: agentfake.ActionFinish})
			},
			Block: func(t *testing.T, s agentrunner.Session, _ string) {
				control(t, s, agentfake.Control{Action: agentfake.ActionBlock})
			},
			RateLimit: func(t *testing.T, s agentrunner.Session, resetAt time.Time) {
				control(t, s, agentfake.Control{Action: agentfake.ActionRateLimit, ResetsAt: resetAt.Unix()})
				// The suite reads RateLimit straight away, with no Wait for the
				// tailer to fold the event in.
				deadline := time.Now().Add(5 * time.Second)
				for {
					if _, limited, err := runner.RateLimit(s); err != nil || limited {
						return
					}
					if time.Now().After(deadline) {
						t.Fatal("runner never saw the rate limit")
					}
					time.Sleep(runner.PollInterval)
				}
			},
			Stall: func(t *testing.T, s agentrunner.Session) {
				control(t, s, agentfake.Control{Action: agentfake.ActionStall})
			},
			BackgroundTask: func(t *testing.T, s agentrunner.Session, running bool) {
				control(t, s, agentfake.Control{Action: agentfake.ActionBackground, Running: running})
			},
			Restart: func(t *testing.T) agentrunner.Runner {
				r := newRunner()
				entries, err := os.ReadDir(root)
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range entries {
					if _, _, err := r.Reattach(e.Name()); err != nil {
						t.Fatalf("Reattach(%s): %v", e.Name(), err)
					}
				}
				return r
			},
		}
	})
}

func killAgents(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(root, e.Name(), nativerunner.MetaFile))
		if err != nil {
			continue
		}
		var meta nativerunner.Meta
		if json.Unmarshal(data, &meta) == nil && meta.PID > 0 {
			_ = syscall.Kill(-meta.PID, syscall.SIGKILL)
		}
	}
}
