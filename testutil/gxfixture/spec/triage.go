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
	"verify", "b-questions", "b-review", "writing", "result", "stopped",
	"journey", "b-group",
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
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) { atApprove(rec, nil) }}}
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
	case "b-questions":
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 3, 3)}
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				atApprove(rec, func(i int, p *triage.PlanRow) {
					if i < 3 {
						p.Questions = []triage.Question{{ID: fmt.Sprintf("q%d", i), Text: "Which epic should this go to?", Kind: triage.QuestionQuick, Options: []string{"ready-epic", "new map"}}}
					}
				})
			}}}
		})
	case "b-review":
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 5, 5)}
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				actions := []string{"ready", "ready", "map", "map", "merge", "merge", "close", "close", "keep", "keep"}
				atApprove(rec, func(i int, p *triage.PlanRow) {
					p.Action = actions[i]
					switch p.Action {
					case "ready", "map":
						p.Destination = fmt.Sprintf("%s-epic", p.Action)
					case "merge":
						p.Destination = rec.Rows[0].Address
					case "close":
						p.Destination = "obsolete"
					}
				})
				// Three rows the person changed from the plan.
				rec.Rows[1].Person.Action = "close"
				rec.Rows[3].Person.Action = "ready"
				rec.Rows[5].Person.Action = "keep"
			}}}
		})
	case "b-group":
		// The first three rows are one group; ungrouped they chain, each after the one before.
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 4, 0)}
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				atApprove(rec, func(i int, p *triage.PlanRow) {
					if i >= 3 {
						p.Action, p.Destination = "keep", ""
						return
					}
					p.Estimate = "10K"
					if i > 0 {
						p.OverlapWith = []string{rec.Rows[i-1].Address}
					}
				})
				for i := range rec.Rows[:3] {
					rec.Rows[i].Group = rec.Rows[0].Address
				}
			}}}
		})
	case "writing":
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 6, 0)}
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				actions := []string{"ready", "ready", "map", "merge", "close", "keep"}
				atApprove(rec, func(i int, p *triage.PlanRow) { p.Action = actions[i] })
				rec.Step = tickets.StepWrite
				rec.Rows[0].Write = triage.WriteDone
				rec.Rows[0].NewAddress = project + ":triage-ready/01"
				// Failed before its move: the follow-up never left follow-ups.
				rec.Rows[1].Write = triage.WriteFailed
				rec.Rows[1].Refusal = "ticket claimed by another agent"
				// Failed after its move: the ticket is already in the new epic.
				rec.Rows[2].Write = triage.WriteFailed
				rec.Rows[2].NewAddress = project + ":triage-map/01"
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
						Write: triage.WriteDone, NewAddress: newAddr,
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
				rec.Step = tickets.StepPlan
			}}}
		})
	case "journey":
		// Verify takes two sessions; at Write the first and fifth rows fail once before their move and the second once after it.
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 3, 2)}
			sc.TriageAgent = &triageplay.Config{
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

// atApprove puts rec at the approve step with a verdict and a plan on every row
// (all "ready" unless shape changes it).
func atApprove(rec *triage.Record, shape func(i int, p *triage.PlanRow)) {
	rec.Step = tickets.StepApprove
	for i := range rec.Rows {
		row := &rec.Rows[i]
		row.Verdict = triage.VerdictRelevant
		row.Plan = &triage.PlanRow{RowID: row.Address, Action: "ready", Destination: "ready-epic"}
		if shape != nil {
			shape(i, row.Plan)
		}
	}
}
