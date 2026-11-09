package spec

// ContextWindowScenario lists the sessions the context-window flows read. The
// keyed ones (Output.Transcripts) take another usage line from a flow.
func ContextWindowScenario() scenario {
	const mapAddr = "design-epic"
	// finish ends the session, which hides its composer.
	finish := func(s sessionSpec) sessionSpec {
		s.Finish = true
		return s
	}
	sc := scenario{
		Codex:       true,
		Chat:        true,
		Epics:       []epic{{project, mapAddr, nil}},
		DesignSteps: map[string]string{project + ":" + mapAddr: "resolve-tickets"},
		Sessions: []sessionSpec{
			{Project: project, Prompt: "hello", Read: true, Tokens: 40_000, Key: "green"},
			{Project: project, Prompt: "hello", Read: true, Tokens: 90_000, Key: "yellow"},
			{Project: project, Prompt: "hello", Read: true, Tokens: 128_000, Key: "orange"},
			{Project: project, Prompt: "hello", Read: true, Tokens: 170_000, Key: "red"},
			{Project: project},
			finish(sessionSpec{Project: project, Prompt: "hello", Read: true, Tokens: 40_000}),
			finish(sessionSpec{Project: project, Prompt: "hello", Read: true, Tokens: 40_000, Link: project + ":" + mapAddr}),
			{Project: project, Prompt: "hello", Read: true, Tokens: 90_000, Steps: []commandStep{{Prompt: "/compact", Turn: true}}, Key: "compact"},
			{Project: project, Prompt: "hello", Read: true, NoTranscript: true},
			{Project: project, Harness: "codex", Prompt: "hello", Read: true, Tokens: 90_000},
		},
	}
	addQueueAgent(&sc)
	addDesignAgents(&sc)
	sc.Epics = append(sc.Epics, followUps(project, 1, 0))
	sc.StepsAfterSessions = append(sc.StepsAfterSessions, []string{"server", "triage", "open", project})
	return sc
}
