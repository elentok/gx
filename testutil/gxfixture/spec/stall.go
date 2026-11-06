package spec

import (
	"slices"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// RejectedFirstPrompt is a server-launched session whose agent rejects the
// slash first prompt as an unknown command, so the session shows a failure.
var RejectedFirstPrompt = scenario{
	UnknownSkill: true,
	Sessions:     []sessionSpec{{Project: project, Prompt: "/gx-implement demo:ready-epic/01", Rejected: true}},
}

// stallNudgeConfig shortens the idle wait before the server nudges a stalled agent.
var stallNudgeConfig = []string{"execution-queue.stalled-nudge-delay=2s"}

// stallEpic is one queued ticket; behaviour decides how its agent stalls.
func stallEpic(name, behaviour string) scenario {
	return scenario{
		Epics:     []epic{{project, name, []ticket{{"01", "first", servertest.TicketOpts{}}}}},
		Queue:     []string{project + ":" + name},
		Behaviour: map[string]string{project + ":" + name + "/01": behaviour},
		Config:    stallNudgeConfig,
	}
}

var (
	// StallVim: the agent goes idle without a report; a test toggles it into vim
	// before the stall nudge arrives.
	StallVim = stallEpic("stall-vim-epic", agentfake.BehaviourStall)
	// StallLeavesI: the same stall, and the agent leaves one `i` after its first clear.
	StallLeavesI = withEnv(stallEpic("stall-i-epic", agentfake.BehaviourStall), agentfake.TUIClearLeavesIEnv+"=1")
	// LateRegistry: the skill registry fills a few seconds after start, so the
	// launch prompt is resent until it lands.
	LateRegistry = withEnv(stallEpic("late-registry-epic", agentfake.BehaviourFinish),
		agentfake.TUIRegistryDelayEnv+"=6s", "GX_LAUNCH_UNKNOWN_GRACE=20s")
	// NoUserRecord: the agent stalls in a conversation with no user record.
	NoUserRecord = stallEpic("no-record-epic", agentfake.BehaviourStallNoRecord)
)

func withEnv(sc scenario, env ...string) scenario {
	sc.Env = append(slices.Clone(sc.Env), env...)
	return sc
}
