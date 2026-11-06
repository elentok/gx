package main

var (
	// rejectedFirstPrompt is a server-launched session whose agent rejects the
	// slash first prompt as an unknown command, so the session shows a failure.
	rejectedFirstPrompt = scenario{
		unknownSkill: true,
		sessions:     []sessionSpec{{project: project, prompt: "/gx-implement demo:ready-epic/01", rejected: true}},
	}
)
