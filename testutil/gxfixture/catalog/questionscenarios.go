package catalog

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/questionseed"
)

// questionOptions is the options block both long-question tickets share.
const questionOptions = `type: options
recommended: b
options:
  - id: a
    label: Keep the current layout
    consequence: Nothing changes and the long question stays hard to read.
  - id: b
    label: Clamp the title and fold the body
    consequence: The page stays short, and a tap opens the full text.
  - id: c
    label: Move the question to its own page
    consequence: Every ask gets room, but answering takes one more tap.
`

// oneOffQuestion is the one-off-question scenario: a One-off (the one-offs
// epic) with one open agent question and no live agent, so the web shows Cancel.
func oneOffQuestion() scenario {
	return scenario{
		Epics: []epic{{Project: project, Name: "one-offs", Tickets: []ticket{
			{ID: "01", Slug: "asks", Opts: servertest.TicketOpts{PauseReason: "self-reported"}},
		}}},
		Questions: map[string]questionseed.Question{
			project + ":one-offs/01": {
				Summary: "Keep the card as it is?",
				Text:    "One line of context.",
				Block:   questionOptions,
				Handoff: "Nothing else changed.",
			},
		},
	}
}

// permissionQuestion is the permission scenario: the agent asks the fake's
// options question, the ticket pauses, then the live agent blocks on a Bash
// permission prompt (agentfake.BehaviourAskPermission).
func permissionQuestion() scenario {
	const addr = project + ":permission-epic/01"
	return scenario{
		Epics: []epic{{Project: project, Name: "permission-epic", Tickets: []ticket{
			{ID: "01", Slug: "asks-then-blocks", Opts: servertest.TicketOpts{}},
		}}},
		Behaviour: map[string]string{addr: agentfake.BehaviourAskPermission},
		Queue:     []string{project + ":permission-epic"},
	}
}

// longQuestion is the long-question scenario: one open agent question with a
// long title, a bullet body and two earlier rounds, and one short one. Its epic
// is not queued, so nothing launches under the paused tickets.
func longQuestion() scenario {
	const addr = project + ":long-question-epic/"
	pause := servertest.TicketOpts{PauseReason: "self-reported"}
	return scenario{
		Epics: []epic{{Project: project, Name: "long-question-epic", Tickets: []ticket{
			{ID: "01", Slug: "long-question", Opts: pause},
			{ID: "02", Slug: "short-question", Opts: pause},
		}}},
		Questions: map[string]questionseed.Question{
			addr + "01": {
				Summary: "Should the queue card clamp a long question title to three lines, or show all of it on a narrow phone screen?",
				Text: "The agent found a few ways to lay out a long ask on a phone:\n\n" +
					"- clamp the title to three lines and show the rest on a tap\n" +
					"- show the whole title and shrink the body text\n" +
					"- split the title from the body with a rule\n" +
					"- keep the options block above the fold\n" +
					"- move the notes under the options",
				Block:   questionOptions,
				Handoff: "I read the queue card and its tests. Both layouts pass them. The clamp keeps the card short.",
				Earlier: []questionseed.Round{
					{
						Summary: "Which heading level should the question use?",
						Text:    "The card has no heading yet.",
						Handoff: "I found two heading levels in the page styles.",
						Reply:   "Use the second level.",
					},
					{
						Summary: "Should the options sit above or below the body?",
						Text:    "The body is short in most asks.",
						Block:   questionOptions,
						Handoff: "Both orders read well in the mock.",
						Choice:  "a",
						Note:    "Below the body, so the context comes first.",
					},
				},
			},
			addr + "02": {
				Summary: "Keep the card as it is?",
				Text:    "One line of context.",
				Block:   questionOptions,
				Handoff: "Nothing else changed.",
			},
		},
	}
}
