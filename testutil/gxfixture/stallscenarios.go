package main

import (
	"slices"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// rejectedFirstPrompt is a server-launched session whose agent rejects the
// slash first prompt as an unknown command, so the session shows a failure.
var rejectedFirstPrompt = scenario{
	unknownSkill: true,
	sessions:     []sessionSpec{{project: project, prompt: "/gx-implement demo:ready-epic/01", rejected: true}},
}

// stallNudgeConfig shortens the idle wait before the server nudges a stalled agent.
var stallNudgeConfig = []string{"execution-queue.stalled-nudge-delay=2s"}

// stallEpic is one queued ticket; behaviour decides how its agent stalls.
func stallEpic(name, behaviour string) scenario {
	return scenario{
		epics:     []epic{{project, name, []ticket{{"01", "first", servertest.TicketOpts{}}}}},
		queue:     []string{project + ":" + name},
		behaviour: map[string]string{project + ":" + name + "/01": behaviour},
		config:    stallNudgeConfig,
	}
}

var (
	// stallVim: the agent goes idle without a report; a test toggles it into vim
	// before the stall nudge arrives.
	stallVim = stallEpic("stall-vim-epic", agentfake.BehaviourStall)
	// stallLeavesI: the same stall, and the agent leaves one `i` after its first clear.
	stallLeavesI = withEnv(stallEpic("stall-i-epic", agentfake.BehaviourStall), agentfake.TUIClearLeavesIEnv+"=1")
	// lateRegistry: the skill registry fills a few seconds after start, so the
	// launch prompt is resent until it lands.
	lateRegistry = withEnv(stallEpic("late-registry-epic", agentfake.BehaviourFinish),
		agentfake.TUIRegistryDelayEnv+"=6s", "GX_LAUNCH_UNKNOWN_GRACE=20s")
	// noUserRecord: the agent stalls in a conversation with no user record.
	noUserRecord = stallEpic("no-record-epic", agentfake.BehaviourStallNoRecord)
)

func withEnv(sc scenario, env ...string) scenario {
	sc.env = append(slices.Clone(sc.env), env...)
	return sc
}
