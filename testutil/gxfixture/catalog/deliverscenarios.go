package catalog

// DeliverCardChats are the first prompts, and so the chat names, of the three
// chats of deliverCardsScenario, in the order of its Sessions.
var DeliverCardChats = [3]string{"not-sent chat", "send-failed chat", "unknown-command chat"}

// deliverLaunchScenario is a project with no session: a test creates the
// session with `gx agents create`, and the server's fake takes one launch knob
// per scenario, so each scenario is one first-prompt outcome (S3 and S4).
func deliverLaunchScenario(knob string) scenario {
	return scenario{Projects: []string{project}, PromptGrace: "2s", Env: []string{knob}}
}

// deliverCardsScenario is three idle Claude chats whose next send ends in one
// of the delivery-failure cards. Each chat takes a control first (agentfake
// ActionUndecided, ActionStall, ActionUnknownSkill), so the flow decides when
// the failure starts.
func deliverCardsScenario() scenario {
	sessions := make([]sessionSpec, len(DeliverCardChats))
	for i, name := range DeliverCardChats {
		sessions[i] = sessionSpec{Project: project, Prompt: name, Read: true}
	}
	// The default grace is 45s; a stalled or undecided send must give up well inside the test's poll.
	return scenario{Projects: []string{project}, Chat: true, PromptGrace: "2s", Sessions: sessions}
}
