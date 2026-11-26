package bgtx

import "time"

// Names of the working card shapes of the subagents scenario, which hold the
// content of the spec's card mock.
const (
	CardTicketsCall = "toolu_card_tickets"
	CardNamesCall   = "toolu_card_names"
	CardProbeCall   = "toolu_card_probe"

	CardTicketsAgent = "a5ub0013"
	CardNamesAgent   = "a5ub0014"
	CardProbeAgent   = "a5ub0015"

	CardTicketsDesc = "Explore tickets package"
	CardNamesDesc   = "Find where task rows lose the name"
	CardProbeDesc   = "Probe Codex rollout format"

	// The one call without a result in each running subagent's file; a flow
	// appends the result to close it.
	CardTicketsOpenCall = "toolu_card_read"
	CardProbeOpenCall   = "toolu_card_bash"

	CardTicketsFileKey = "subagent-card-tickets-file"
	CardTicketsMetaKey = "subagent-card-tickets-meta"
	CardNamesFileKey   = "subagent-card-names-file"
	CardNamesMetaKey   = "subagent-card-names-meta"
	CardProbeFileKey   = "subagent-card-probe-file"
	CardProbeMetaKey   = "subagent-card-probe-meta"

	cardNamesHandback = "The label is dropped when the row is rebuilt."
	cardTicketsAgo    = 7*minute + 34*sec
)

// cardTickets is a running subagent with an open Read.
func (b *Lines) cardTickets(ago time.Duration) {
	file := []string{
		subAssistant("msg_ct1", 900, 40, toolUse("toolu_card_glob", "Glob", map[string]any{"pattern": "tickets/*.go"})),
		subResult("toolu_card_glob", "tickets/frontier.go", nil),
		subAssistant("msg_ct2", 1300, 60, toolUse(CardTicketsOpenCall, "Read", map[string]any{"file_path": "tickets/frontier.go"})),
	}
	b.subagentCall(ago, CardTicketsCall, CardTicketsAgent, CardTicketsDesc, false, file, CardTicketsFileKey, CardTicketsMetaKey)
}

// cardNames is a subagent that finished after took.
func (b *Lines) cardNames(ago, took time.Duration) {
	file := []string{
		subAssistant("msg_cn1", 800, 50, toolUse("toolu_card_grep", "Grep", map[string]any{"pattern": "Label"})),
		subResult("toolu_card_grep", "server/sessions/chat/tasks.go", nil),
		subAssistant("msg_cn2", 1200, 70, toolUse("toolu_card_handback", "SubagentHandback", map[string]any{"message": cardNamesHandback})),
		subResult("toolu_card_handback", "Handback delivered", nil),
	}
	b.subagentCall(ago, CardNamesCall, CardNamesAgent, CardNamesDesc, true, file, CardNamesFileKey, CardNamesMetaKey)
	b.result(ago-took, CardNamesCall, cardNamesHandback, map[string]any{"status": "completed", "agentId": CardNamesAgent, "resolvedModel": model})
}

// cardProbe is a running subagent with an open Bash call.
func (b *Lines) cardProbe(ago time.Duration) {
	file := []string{
		subAssistant("msg_cp1", 700, 30, toolUse(CardProbeOpenCall, "Bash", map[string]any{"command": "rg -n spawn_agent codexsession"})),
	}
	b.subagentCall(ago, CardProbeCall, CardProbeAgent, CardProbeDesc, false, file, CardProbeFileKey, CardProbeMetaKey)
}

// cardTwoRunning is two running subagents around one that finished.
func cardTwoRunning(b *Lines) {
	b.cardTickets(cardTicketsAgo)
	b.cardNames(6*minute, 4*minute+51*sec)
	b.cardProbe(2*minute + 8*sec)
}
