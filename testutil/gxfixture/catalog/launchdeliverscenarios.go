package catalog

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// launchDeliverScenario is a queued epic with one ticket whose launch prompt the
// fake answers by one knob, under a short launch window so the unattended launch
// gives up in seconds. It is its own scenario because the knob is an env var
// that every agent of a server shares.
func launchDeliverScenario(epicName, window string, knob string, more ...string) scenario {
	return scenario{
		Epics: []epic{{Project: project, Name: epicName, Tickets: []ticket{{ID: "01", Slug: "first", Opts: servertest.TicketOpts{}}}}},
		Queue: []string{project + ":" + epicName},
		Env:   append([]string{knob, "GX_LAUNCH_UNKNOWN_GRACE=" + window}, more...),
	}
}

var (
	// launch-rejected (S6): every launch prompt is answered "Unknown command:",
	// so the launches, the skill refresh and the last launch all fail and the
	// ticket pauses as agent_prompt_stalled.
	// The window must exceed the retype interval, so a retype fits inside it; a
	// short interval keeps the three launches under the slow-test limit.
	launchRejected = launchDeliverScenario("launch-rejected-epic", "2s", agentfake.TUIUnknownSkillEnv+"=1", "GX_LAUNCH_RETYPE_EVERY=500ms")
	// launch-undecided (S7): the launch prompt stays in the input box and no
	// Enter lands, so delivery is undecided and nothing is retyped.
	launchUndecided = launchDeliverScenario("launch-undecided-epic", "2s", agentfake.TUIUndecidedEnv+"=1")
)
