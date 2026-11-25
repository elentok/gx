package spec

import (
	"fmt"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake/triageplay"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketmove"
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
	"verify", "route", "route-12-backlogs", "route-no-backlog", "questions", "final-approval",
	"writing", "result", "stopped", "journey", "backlog", "old-epics",
}

// oldEpicsProject is the project of the old-epics shape: the shipped backlog
// table is keyed by project and old-epic slug.
const oldEpicsProject = "gx"

// backlogEpic is a backlog with n open tickets (draft and open alternating), from
// id first on, and claimed more that are not open.
func backlogEpic(sc *Scenario, name string, first, n, claimed int) {
	e := Epic{Project: project, Name: name}
	add := func(opts servertest.TicketOpts) {
		id := fmt.Sprintf("%02d", first+len(e.Tickets))
		e.Tickets = append(e.Tickets, Ticket{id, fmt.Sprintf("%s-%s", name, id), opts})
	}
	for i := range n {
		status := "open"
		if i%2 == 1 {
			status = "draft"
		}
		add(servertest.TicketOpts{Status: status})
	}
	for range claimed {
		add(servertest.TicketOpts{Status: "claimed", ClaimedAt: time.Now().UTC().Format(time.RFC3339)})
	}
	sc.Epics = append(sc.Epics, e)
	sc.EpicMeta[project+":"+name] = "status: open\nkind: backlog\n"
}

// backlogScenario seeds three backlogs with 11, 5 and 0 open tickets and, in the
// five-ticket one, a half-filled charting map for "Finish move".
func backlogScenario(sc *Scenario) {
	sc.EpicMeta = map[string]string{}
	sc.EpicBody = map[string]string{}
	backlogEpic(sc, "parser", 1, 11, 1)
	// Tickets 01 and 02 already moved into the map; 03 and 04 are still here.
	backlogEpic(sc, "sync", 3, 5, 0)
	backlogEpic(sc, "webui", 1, 0, 0)
	mapAddr := project + ":retry-work"
	sc.Epics = append(sc.Epics, Epic{project, "retry-work", []Ticket{
		{"01", "moved-01", servertest.TicketOpts{}}, {"02", "moved-02", servertest.TicketOpts{}},
	}})
	sc.EpicMeta[mapAddr] = "status: open\nkind: map\ndestination_unconfirmed: true\n"
	sc.EpicBody[mapAddr] = "# Retry work\n\n## Destination\n\nA provisional Destination for the \"Retry work\" theme of the \"sync\" backlog.\n\n## Notes\n\n" +
		ticketmove.StartDesignNotes(project, "sync", []string{"01", "02", "03", "04"})
}

// oldEpicsScenario seeds old triage-made maps in project gx and no backlog, plus
// a follow-up epic so a Triage starts and its Route plans the moves.
func oldEpicsScenario(sc *Scenario, open func(string) []string) {
	sc.EpicMeta = map[string]string{}
	sc.EpicBody = map[string]string{}
	oldEpic := func(name, meta string, ticketOpts ...servertest.TicketOpts) {
		e := Epic{Project: oldEpicsProject, Name: name}
		for i, opts := range ticketOpts {
			id := fmt.Sprintf("%02d", i+1)
			e.Tickets = append(e.Tickets, Ticket{id, name + "-" + id, opts})
		}
		sc.Epics = append(sc.Epics, e)
		addr := oldEpicsProject + ":" + name
		sc.EpicMeta[addr] = "status: open\nkind: map\n" + meta
		sc.EpicBody[addr] = "# " + name + "\n\n## Notes\n\n" + triage.OldEpicNote + "\n"
	}
	ready := servertest.TicketOpts{}
	oldEpic("test-flakes", "", ready, servertest.TicketOpts{Status: "draft"})
	oldEpic("agent-guidance", "", ready)
	oldEpic("ticket-authoring", "", ready)
	oldEpic("scratch-ideas", "", ready) // not in the shipped table
	oldEpic("tickets-cli", "design_step: spec\n", ready)
	oldEpic("gx-check", "") // empty
	oldEpic("stuck-ticket-watchdog", "", servertest.TicketOpts{Status: "claimed", ClaimedAt: time.Now().UTC().Format(time.RFC3339)})
	sc.Epics = append(sc.Epics, followUps(oldEpicsProject, 4, 2))
	sc.Triage = []TriageSeed{{oldEpicsProject, func(rec *triage.Record) {
		for i := range rec.Rows {
			rec.Rows[i].Verdict = triage.VerdictRelevant
		}
		rec.Step = tickets.StepRoute
	}}}
	sc.Steps = [][]string{open(oldEpicsProject)}
}

// triageBacklogNames are the backlog epics a shape seeds, first n for n backlogs.
var triageBacklogNames = []string{
	"parser", "sync", "webui", "cli", "server", "docs",
	"build", "ci", "tests", "config", "storage", "auth",
}

// withBacklogs seeds the first n backlogs of triageBacklogNames as backlog epics.
func withBacklogs(sc *Scenario, n int) []string {
	names := triageBacklogNames[:n]
	if sc.EpicMeta == nil {
		sc.EpicMeta = map[string]string{}
	}
	for _, name := range names {
		sc.Epics = append(sc.Epics, Epic{project, name, nil})
		sc.EpicMeta[project+":"+name] = "status: open\nkind: backlog\n"
	}
	return names
}

// HoldTriage makes the scenario's Triage session wait for the hold file before
// it plays its step, so a `--hold` run stays at the shape's step.
func HoldTriage(sc *Scenario) {
	cfg := triageplay.Config{}
	if sc.TriageAgent != nil {
		cfg = *sc.TriageAgent
	}
	cfg.HoldAtStep = true
	sc.TriageAgent = &cfg
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
	case "backlog":
		return with(backlogScenario)
	case "old-epics":
		return with(func(sc *Scenario) { oldEpicsScenario(sc, open) })
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
			// One row per session leaves the rest pending, so the page stays at Verify. The session waits on the hold, so the page is read while Verify is working.
			sc.TriageAgent = &triageplay.Config{VerifyBatch: 1, HoldAtStep: true}
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				for i := range rec.Rows[:31] {
					rec.Rows[i].Verdict = triage.VerdictRelevant
				}
			}}}
			sc.Steps = [][]string{open(project)}
		})
	case "route", "route-12-backlogs", "route-no-backlog":
		// Verdicts saved and no plan: the session plays Route, which saves the plan.
		return with(func(sc *Scenario) {
			n := map[string]int{"route": 3, "route-12-backlogs": tickets.MaxBacklogs, "route-no-backlog": 0}[shape]
			withBacklogs(sc, n)
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
			backlogs := withBacklogs(sc, 3)
			sc.Epics = append(sc.Epics, followUps(project, 4, 2))
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				answered := shape == "final-approval"
				atQuestions(rec, backlogs, answered)
			}}}
			sc.Steps = [][]string{open(project)}
		})
	case "writing":
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 6, 0)}
			// The scene is the page mid-Write with three rows still pending: the
			// session waits on the hold so it does not write them.
			sc.TriageAgent = &triageplay.Config{HoldAtStep: true}
			sc.Triage = []TriageSeed{{project, func(rec *triage.Record) {
				actions := []string{"ready", "ready", "backlog", "merge", "close", "keep"}
				planned(rec, func(i int, p *triage.PlanRow) {
					p.Action = actions[i]
					// A merge names the row it duplicates and a close names its reason.
					switch p.Action {
					case "merge":
						p.Destination = rec.Rows[0].Address
					case "close":
						p.Destination = "stale"
					}
				})
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
					moved(3, "backlog", mapEpic+"/01"), moved(4, "close", ""),
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
		// Verify takes two sessions, no backlog exists, and the plan's ready rows are 01 and 04: at Write the first fails once before its move and the fourth once after it.
		return with(func(sc *Scenario) {
			sc.Epics = []Epic{followUps(project, 3, 2)}
			sc.TriageAgent = &triageplay.Config{
				VerifyBatch:    3,
				FailBeforeMove: []string{project + ":" + tickets.FollowUpsEpic + "/01"},
				FailAfterMove:  []string{project + ":" + tickets.FollowUpsEpic + "/04"},
			}
		})
	}
	return Scenario{}, false
}

// FollowUps is followUps for the scenarios in sub-packages.
var FollowUps = followUps

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
// saves, the new backlog proposed (while there is room for it) and the first
// round started: the new-backlog question and the first ask question. With
// answered, every ask question is in a round and every round is answered.
func atQuestions(rec *triage.Record, backlogs []string, answered bool) {
	rec.Step = tickets.StepQuestions
	plan := triageplay.RoutePlan(rec, backlogs)
	for i := range rec.Rows {
		p := plan[i]
		rec.Rows[i].Verdict = triage.VerdictRelevant
		rec.Rows[i].Plan = &p
		rec.Rows[i].Fallback = p.Fallback
	}
	if len(backlogs) < tickets.MaxBacklogs {
		rec.Backlogs = []triage.Backlog{{Name: triageplay.NewBacklog, Source: triage.BacklogAgent, Confirmed: answered}}
	}
	rec.Decided = triageplay.Decisions(rec, backlogs)
	round := triage.Round{Answers: map[string]string{}}
	if len(backlogs) < tickets.MaxBacklogs {
		q := triageplay.NewBacklogQuestion(rec, backlogs)
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
