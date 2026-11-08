package spec

import (
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/supervise"
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
	// SlowHolder: the server's first three holder status calls get no dial
	// answer, so the ticket card shows "holder unreachable, retrying" and
	// clears once a dial works (under HolderReadoptCap, so it never pauses).
	SlowHolder = recoveryEpic("slow-holder-epic", servertest.TicketOpts{}, false,
		holder.UnreachableStatusDialsEnv+"=3")
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
	// ClassifierOutage: every turn ends on a "no verdict" tool error, so the
	// re-sends run out and the ticket pauses as classifier-outage.
	ClassifierOutage = recoveryEpic("classifier-outage-epic", servertest.TicketOpts{}, false,
		agentfake.TUIClassifierOutageEnv+"=1", ralphloop.ClassifierResendDelayEnv+"=1ms")
	// HolderUnreachable: every holder status call gets no dial answer, so past
	// HolderReadoptCap the ticket pauses as iteration-error with the holder alive.
	HolderUnreachable = recoveryEpic("holder-dead-epic", servertest.TicketOpts{}, false,
		holder.UnreachableStatusDialsEnv+"=100000")
)

// backgroundEpic is recoveryEpic whose agent runs behaviour on its ticket, and
// whose live-task hold cap is short enough to reach the pause, yet long enough
// that a recovery shows the pause cleared before the relaunched agent hits it again.
func backgroundEpic(name string, opts servertest.TicketOpts, codexHarness bool, behaviour string) scenario {
	sc := recoveryEpic(name, opts, codexHarness, supervise.BackgroundTaskHoldCapEnv+"=45s")
	sc.Behaviour = map[string]string{project + ":" + name + "/01": behaviour}
	return sc
}
