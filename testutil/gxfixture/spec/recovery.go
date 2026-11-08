package spec

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/agentfake/codex"
)

// RootToken in a scenario's Env value stands for the fixture's root dir, so a
// knob that names a file (the hang counter) can point inside it.
const RootToken = "{root}"

// recoveryEpic is one queued ticket whose agent runs under the given knobs.
func recoveryEpic(name string, opts servertest.TicketOpts, codexHarness bool, env ...string) scenario {
	return scenario{
		Epics:  []epic{{project, name, []ticket{{"01", "first", opts}}}},
		Queue:  []string{project + ":" + name},
		Codex:  codexHarness,
		Env:    env,
		Config: stallNudgeConfig,
	}
}

var (
	// HungPaneTwice: the first two launches hang (the pane echoes but takes no
	// key), so the one relaunch also hangs and the ticket pauses as
	// agent_prompt_stalled.
	HungPaneTwice = recoveryEpic("hung-epic", servertest.TicketOpts{}, false,
		agentfake.TUIHangLaunchesEnv+"=2:"+RootToken+"/hang-count")
	// HungPaneOnce: only the first launch hangs; the relaunch is healthy.
	HungPaneOnce = recoveryEpic("hung-once-epic", servertest.TicketOpts{}, false,
		agentfake.TUIHangLaunchesEnv+"=1:"+RootToken+"/hang-count")
	// HungPaneTwiceCodex is HungPaneTwice on the Codex fake.
	HungPaneTwiceCodex = recoveryEpic("hung-codex-epic", servertest.TicketOpts{Agent: "codex"}, true,
		agentfake.TUIHangLaunchesEnv+"=2:"+RootToken+"/hang-count")
	// CodexDropsPrompt: the Codex fake never takes its launch prompt, so the
	// launch proof from the rollout fails.
	CodexDropsPrompt = recoveryEpic("codex-drop-epic", servertest.TicketOpts{Agent: "codex"}, true,
		codex.CodexDropPromptEnv+"=1")
	// LiveTaskGate: the Claude agent ends its turn on a background task that has
	// an output file and a running child process, so the ticket pauses on the
	// live-task gate.
	LiveTaskGate = backgroundEpic("live-task-epic", servertest.TicketOpts{}, false, agentfake.BehaviourBackgroundTask)
	// LiveTaskGateCodex is the same gate on the Codex fake, which shows only its
	// background-terminal title.
	LiveTaskGateCodex = backgroundEpic("live-task-codex-epic", servertest.TicketOpts{Agent: "codex"}, true, agentfake.BehaviourBackgroundTask)
	// LiveTaskGateDirty is LiveTaskGate with an uncommitted file in the worktree.
	LiveTaskGateDirty = backgroundEpic("live-task-dirty-epic", servertest.TicketOpts{}, false, agentfake.BehaviourBackgroundDirty)
)

// backgroundEpic is recoveryEpic whose agent runs behaviour on its ticket.
func backgroundEpic(name string, opts servertest.TicketOpts, codexHarness bool, behaviour string) scenario {
	sc := recoveryEpic(name, opts, codexHarness)
	sc.Behaviour = map[string]string{project + ":" + name + "/01": behaviour}
	return sc
}
