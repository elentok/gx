package spec

import (
	"fmt"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake/triageplay"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/triage"
)

// TriageSide is the second project of the shapes that need two.
const TriageSide = "side"

// TriageWorkspace is the named workspace of the c-named shape.
const TriageWorkspace = "triage ws"

// TriageSeed is a Triage record written with the triage package before the
// server starts: Create snapshots the project's follow-ups, Edit shapes the
// record and it is saved.
type TriageSeed struct {
	Project string
	Edit    func(rec *triage.Record)
}

// TriageShapes are the --shape names of the triage scenario, named after the
// mock states they stand for.
var TriageShapes = []string{
	"c-zero", "c-few", "c-many", "c-live", "c-named",
	"verify", "route", "route-12-areas", "route-no-area", "questions", "final-approval",
	"writing", "result", "stopped", "journey",
}

// triageAreaNames are the area epics a shape seeds, first n for n areas.
var triageAreaNames = []string{
	"parser", "sync", "webui", "cli", "server", "docs",
	"build", "ci", "tests", "config", "storage", "auth",
}

// withAreas seeds the first n areas of triageAreaNames as area epics.
func withAreas(sc *Scenario, n int) []string {
	names := triageAreaNames[:n]
	if sc.EpicMeta == nil {
		sc.EpicMeta = map[string]string{}
	}
	for _, name := range names {
		sc.Epics = append(sc.Epics, Epic{project, name, nil})
		sc.EpicMeta[project+":"+name] = "status: open\nkind: area\n"
	}
	return names
}

// TriageScenario is the triage scenario in the given shape. Each shape is its
// own server start: the sidebar badge sums every project of a workspace.
func TriageScenario(shape string) (Scenario, bool) {
	open := func(project string) []string { return []string{"server", "triage", "open", project} }
	base := Scenario{Chat: true, Hold: true}
	with := func(f func(sc *Scenario)) (Scenario, bool) {
		sc := base
		f(&sc)
		return sc, true
	}
	switch shape {
	case "c-zero":
		return with(func(sc *Scenario) { sc.Projects = []string{project} })
	case "c-few":
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 1, 1), followUps(TriageSide, 1, 0)}
		})
	case "c-many":
		return with(func(sc *Scenario) { sc.Epics = []Epic{followUps(project, 30, 15)} })
	case "c-live":
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 6, 6)}
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				planned(rec, nil)
				rec.Step = tickets.StepQuestions
			}}}
		})
	case "c-named":
		return with(func(sc *Scenario) {
			readyEpic := func(p string) Epic {
				return Epic{p, "ready-epic", []Ticket{{"01", "first", servertest.TicketOpts{}}}}
			}
			sc.Epics = []Epic{followUps(project, 2, 0), readyEpic(project), followUps(TriageSide, 2, 0), readyEpic(TriageSide)}
			sc.Steps = [][]string{
				{"server", "workspaces", "create", TriageWorkspace},
				{"server", "workspaces", "move", project + ":ready-epic", TriageWorkspace},
				{"server", "workspaces", "move", TriageSide + ":ready-epic", TriageWorkspace},
			}
		})
	case "verify":
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 30, 15)}
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				for i := range rec.Rows[:31] {
					rec.Rows[i].Verdict = triage.VerdictRelevant
				}
			}}}
			sc.Steps = [][]string{open(project)}
		})
	case "route", "route-12-areas", "route-no-area":
		// Verdicts saved and no plan: the session plays Route, which saves the plan.
		return with(func(sc *Scenario) {
			n := map[string]int{"route": 3, "route-12-areas": tickets.MaxAreas, "route-no-area": 0}[shape]
			withAreas(sc, n)
			sc.Epics = append(sc.Epics, followUps(project, 4, 2))
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				for i := range rec.Rows {
					rec.Rows[i].Verdict = triage.VerdictRelevant
				}
				rec.Step = tickets.StepRoute
			}}}
			sc.Steps = [][]string{open(project)}
		})
	case "questions", "final-approval":
		// The plan is saved and one round was started: it waits (questions) or
		// every round is answered and the final card waits (final-approval).
		return with(func(sc *Scenario) {
			areas := withAreas(sc, 3)
			sc.Epics = append(sc.Epics, followUps(project, 4, 2))
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				answered := shape == "final-approval"
				atQuestions(rec, areas, answered)
			}}}
			sc.Steps = [][]string{open(project)}
		})
	case "writing":
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 6, 0)}
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				actions := []string{"ready", "ready", "map", "merge", "close", "keep"}
				planned(rec, func(i int, p *triage.PlanRow) { p.Action = actions[i] })
				rec.Step = tickets.StepWrite
				rec.Rows[0].Write = triage.WriteDone
				rec.Rows[0].NewAddresses = []string{project + ":triage-ready/01"}
				// Failed before its move: the follow-up never left follow-ups.
				rec.Rows[1].Write = triage.WriteFailed
				rec.Rows[1].Refusal = "ticket claimed by another agent"
				// Failed after its move: the ticket is already in the new epic.
				rec.Rows[2].Write = triage.WriteFailed
				rec.Rows[2].NewAddresses = []string{project + ":triage-map/01"}
				rec.Rows[2].Refusal = "epic add failed"
			}}}
			sc.Steps = [][]string{open(project)}
		})
	case "result":
		return with(func(sc *Scenario) {
			ready, mapEpic := project+":triage-ready", project+":triage-map"
			sc.Epics = []Epic{
				followUps(project, 2, 0),
				{project, "triage-ready", []Ticket{{"01", "first", servertest.TicketOpts{}}, {"02", "second", servertest.TicketOpts{}}}},
				{project, "triage-map", nil},
			}
			sc.EpicMeta = map[string]string{mapEpic: "status: open\nkind: map\n"}
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				for i := range rec.Rows {
					rec.Rows[i].Plan = &triage.PlanRow{RowID: rec.Rows[i].Address, Action: "keep"}
					rec.Rows[i].Write = triage.WriteDone
				}
				moved := func(n int, action, newAddr string) triage.Row {
					title := fmt.Sprintf("Moved follow-up %d", n)
					return triage.Row{
						Address: fmt.Sprintf("%s:follow-ups/%02d", project, 50+n), Title: title, Category: "bug",
						Verdict: triage.VerdictRelevant, Plan: &triage.PlanRow{RowID: title, Action: action},
						Write: triage.WriteDone, NewAddresses: []string{newAddr},
					}
				}
				rec.Rows = append(rec.Rows,
					moved(1, "ready", ready+"/01"), moved(2, "ready", ready+"/02"),
					moved(3, "map", mapEpic+"/01"), moved(4, "close", ""),
				)
				rec.State, rec.Step = triage.StateDone, string(tickets.StepDone)
				rec.LastPassComplete = true
				rec.ReadyEpic, rec.MapEpics = ready, []string{mapEpic}
			}}}
		})
	case "stopped":
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 4, 0)}
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				for i := range rec.Rows {
					rec.Rows[i].Verdict = triage.VerdictRelevant
				}
				rec.Step = tickets.StepRoute
			}}}
		})
	case "journey":
		// Verify takes two sessions; at Write the first and fifth rows fail once before their move and the second once after it.
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 3, 2)}
			sc.TriageAgent = &triageplay.Config{
				LegacyPlan:     true,
				VerifyBatch:    3,
				FailBeforeMove: []string{project + ":" + tickets.FollowUpsEpic + "/01", project + ":" + tickets.FollowUpsEpic + "/05"},
				FailAfterMove:  []string{project + ":" + tickets.FollowUpsEpic + "/02"},
			}
		})
	}
	return Scenario{}, false
}

// followUps is a project's follow-ups epic with open bug and idea tickets.
func followUps(project string, bugs, ideas int) Epic {
	e := Epic{Project: project, Name: tickets.FollowUpsEpic}
	add := func(category string, n int) {
		for range n {
			id := fmt.Sprintf("%02d", len(e.Tickets)+1)
			// Stamped like a ticket made by `tickets add`, so a Triage finds it again after a move.
			address := fmt.Sprintf("%s:%s/%s", project, tickets.FollowUpsEpic, id)
			e.Tickets = append(e.Tickets, Ticket{id, fmt.Sprintf("%s-%s", category, id), servertest.TicketOpts{Category: category, OriginalAddress: address}})
		}
	}
	add("bug", bugs)
	add("idea", ideas)
	return e
}

// atQuestions puts rec at the questions step with the plan the fake Route
// saves, the new area proposed (while there is room for it) and the first
// round started: the new-area question and the first ask question. With
// answered, every ask question is in a round and every round is answered.
func atQuestions(rec *triage.Record, areas []string, answered bool) {
	rec.Step = tickets.StepQuestions
	plan := triageplay.RoutePlan(rec, areas)
	for i := range rec.Rows {
		p := plan[i]
		rec.Rows[i].Verdict = triage.VerdictRelevant
		rec.Rows[i].Plan = &p
		rec.Rows[i].Fallback = p.Fallback
	}
	if len(areas) < tickets.MaxAreas {
		rec.Areas = []triage.Area{{Name: triageplay.NewArea, Source: triage.AreaAgent, Confirmed: answered}}
	}
	rec.Decided = triageplay.Decisions(rec, areas)
	round := triage.Round{Answers: map[string]string{}}
	if len(areas) < tickets.MaxAreas {
		q := triageplay.NewAreaQuestion(rec, areas)
		round.Questions = append(round.Questions, q)
		if answered {
			round.Answers[q.ID] = q.Default
		}
	}
	asks := rec.Unrounded()
	for i, q := range asks {
		if i > 0 && !answered {
			break
		}
		round.Questions = append(round.Questions, q)
		if i == 0 || answered {
			round.Answers[q.ID] = q.Default
		}
	}
	round.Answered = answered
	rec.Rounds = []triage.Round{round}
}

// planned gives every row a verdict and a plan (all "ready" unless shape
// changes it).
func planned(rec *triage.Record, shape func(i int, p *triage.PlanRow)) {
	for i := range rec.Rows {
		row := &rec.Rows[i]
		row.Verdict = triage.VerdictRelevant
		row.Plan = &triage.PlanRow{RowID: row.Address, Action: "ready", Destination: "ready-epic"}
		if shape != nil {
			shape(i, row.Plan)
		}
	}
}
