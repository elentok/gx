package spec

import (
	"slices"
	"strconv"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets/schema"
)

// RejectedFirstPrompt is a server-launched session whose agent rejects the
// slash first prompt as an unknown command, so the session shows a failure.
var RejectedFirstPrompt = scenario{
	UnknownSkill: true,
	// The server retypes a rejected launch prompt for 30s; the seeder waits for the card.
	Env:      []string{"GX_LAUNCH_TIMINGS=3s"},
	Sessions: []sessionSpec{{Project: project, Prompt: "/gx-implement demo:ready-epic/01", Rejected: true}},
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

func longVerifyEpic() scenario {
	var verified []schema.VerifiedEntry
	for i := range 40 {
		verified = append(verified, schema.VerifiedEntry{Story: strconv.Itoa(i + 1), Verdict: "pass", Commit: "deadbeef"})
	}
	const name = "long-verify-epic"
	return scenario{
		Epics:     []epic{{project, name, []ticket{{"01", "verify", servertest.TicketOpts{Type: "verify", Verified: verified}}}}},
		Queue:     []string{project + ":" + name},
		Behaviour: map[string]string{project + ":" + name + "/01": agentfake.BehaviourFinish},
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
	// LongVerifyNote: a verify ticket with many recorded verdicts, so its
	// relaunch note is far longer than the paste limit the fake TUI enforces.
	// The skill runs only when the typed prompt stays the bare command.
	LongVerifyNote = withEnv(longVerifyEpic(), agentfake.TUIPasteOverEnv+"=300")
	// NoUserRecord: the agent stalls in a conversation with no user record.
	NoUserRecord = stallEpic("no-record-epic", agentfake.BehaviourStallNoRecord)
)

func withEnv(sc scenario, env ...string) scenario {
	sc.Env = append(slices.Clone(sc.Env), env...)
	return sc
}
