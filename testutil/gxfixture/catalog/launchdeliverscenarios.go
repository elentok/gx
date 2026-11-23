package catalog

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// launchDeliverScenario is a queued epic with one ticket whose launch prompt the
// fake answers by one knob, under a short launch window so the unattended launch
// gives up in seconds. It is its own scenario because the knob is an env var
// that every agent of a server shares.
func launchDeliverScenario(epicName, window string, knob string) scenario {
	return scenario{
		Epics: []epic{{Project: project, Name: epicName, Tickets: []ticket{{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}}}}},
		Queue: []string{project + ":" + epicName},
		Env:   []string{knob, "GX_LAUNCH_UNKNOWN_GRACE=" + window},
	}
}

var (
	// launch-rejected (S6): every launch prompt is answered "Unknown command:",
	// so the launches, the skill refresh and the last launch all fail and the
	// ticket pauses as agent_prompt_stalled.
	// The window must exceed the 3s retype interval, so a retype fits inside it.
	launchRejected = launchDeliverScenario("launch-rejected-epic", "4s", agentfake.TUIUnknownSkillEnv+"=1")
	// launch-undecided (S7): the launch prompt stays in the input box and no
	// Enter lands, so delivery is undecided and nothing is retyped.
	launchUndecided = launchDeliverScenario("launch-undecided-epic", "2s", agentfake.TUIUndecidedEnv+"=1")
)
