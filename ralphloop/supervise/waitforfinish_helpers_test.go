package supervise

import (
	"os"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/testutil/runnerfake"
)

// withProcessEnv is the process environment with a fake herdr's entries
// layered over it, for DepsOverrides.Env.
func withProcessEnv(fakeEnv []string) []string {
	return append(os.Environ(), fakeEnv...)
}

// blockedRunner is idleRunner with its session blocked.
func blockedRunner(label string) *runnerfake.Runner {
	r := fakes.IdleRunner(label)
	r.SetState(label, agentrunner.StateBlocked, "")
	return r
}

// gatedBreachParams is the launchAndPromptParams shape the compaction-gate
// tests below share: a Claude iteration whose events merge in scratchDir.
func gatedBreachParams(scratchDir string) LaunchParams {
	return LaunchParams{
		Label:      "iter-19",
		Agent:      agent.AgentClaude,
		Pane:       "pane-1",
		Ticket:     "19",
		SessionCwd: "/repo/iter-19",
		ScratchDir: scratchDir,
		EpicName:   "epic",
	}
}

// breachRunner hosts gatedBreachParams' session. Its agent reports idle the
// moment it is prompted, and timeoutIf scripts which compact polls time out.
func breachRunner(timeoutIf func(wait int) bool) *fakes.BlipRunner {
	return &fakes.BlipRunner{Runner: fakes.IdlePromptRunner("iter-19", "sess-19"), TimeoutIf: timeoutIf}
}

// boundGiveUpParams is the iteration shape the give-up-bound tests share: a
// Claude iteration whose occupancy stays over the smart zone, so every poll
// tick that times out drives one full recovery attempt.
func boundGiveUpParams() LaunchParams {
	return LaunchParams{
		Label: "iter-19", Agent: agent.AgentClaude, Pane: "pane-1", Ticket: "19",
		SessionCwd: "/repo/iter-19", SmartZone: 100, Gate: NewGate(),
	}
}

func alwaysTimeout(int) bool { return true }
