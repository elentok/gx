package ralphloop

import (
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/supervise"
	"github.com/elentok/gx/testutil/runnerfake"
)

// blockedRunner is idleRunner with its session blocked.
func blockedRunner(label string) *runnerfake.Runner {
	r := fakes.IdleRunner(label)
	r.SetState(label, agentrunner.StateBlocked, "")
	return r
}

// idlePromptRunner is paneRunner on pane-1 whose agent reports idle the moment
// it is prompted, so a wait after "/compact" never blocks in real time.
func idlePromptRunner(label, sessionID string) *runnerfake.Runner {
	r := fakes.PaneRunner(label, "pane-1", sessionID)
	r.PromptState = agentrunner.StateIdle
	return r
}

// gatedBreachParams is the launchAndPromptParams shape the compaction-gate
// tests below share: a Claude iteration whose events merge in scratchDir.
func gatedBreachParams(scratchDir string) supervise.LaunchParams {
	return supervise.LaunchParams{
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
func breachRunner(timeoutIf func(wait int) bool) *blipRunner {
	return &blipRunner{Runner: idlePromptRunner("iter-19", "sess-19"), timeoutIf: timeoutIf}
}

// boundGiveUpParams is the iteration shape the give-up-bound tests share: a
// Claude iteration whose occupancy stays over the smart zone, so every poll
// tick that times out drives one full recovery attempt.
func boundGiveUpParams() supervise.LaunchParams {
	return supervise.LaunchParams{
		Label: "iter-19", Agent: agent.AgentClaude, Pane: "pane-1", Ticket: "19",
		SessionCwd: "/repo/iter-19", SmartZone: 100, Gate: supervise.NewGate(),
	}
}

// boundGiveUpRunner hosts boundGiveUpParams' session; its finish polls time
// out wherever timeoutIf says. Recovery's own compact-completion polls see the
// agent idle right after "/compact": the premature idle the gate exists to
// distrust.
func boundGiveUpRunner(timeoutIf func(wait int) bool) *blipRunner {
	return &blipRunner{Runner: idlePromptRunner("iter-19", "sess-19"), timeoutIf: timeoutIf, finishPollsOnly: true}
}

func alwaysTimeout(int) bool { return true }
