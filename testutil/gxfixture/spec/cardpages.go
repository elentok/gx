package spec

import (
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/bgtx"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/triage"
)

// CardPagesScenario is the card-pages scenario: three Working Claude sessions,
// each with a running and a finished subagent, on the Design page of
// design-epic, the Triage page and neither. The Triage one also asks a round, so
// the Design page has none open.
func CardPagesScenario() Scenario {
	const mapAddr = project + ":design-epic"
	session := func(key, link, round string) SessionSpec {
		return bgClaudeSession(link, key, agentfake.ChatSlowMarker+" "+key, 2, bgSeed(bgtx.CardPage(round), nil))
	}
	triaged := session("card-triage", "", agentfake.DesignRoundText(agentfake.DesignTurnMixed))
	triaged.Triage = project
	return Scenario{
		Epics:       []Epic{{Project: project, Name: "design-epic"}, followUps(project, 6, 6)},
		DesignSteps: map[string]string{mapAddr: string(tickets.StepResolveTickets)},
		Chat:        true,
		Triage: []TriageSeed{{project, func(rec *triage.Record) {
			planned(rec, nil)
			rec.Step = tickets.StepQuestions
		}}},
		Sessions: []SessionSpec{session("card-design", mapAddr, ""), triaged, session("card-plain", "", "")},
	}
}
