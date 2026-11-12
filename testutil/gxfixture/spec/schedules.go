package spec

import (
	"time"

	"github.com/elentok/gx/server/schedules"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
)

// SchedulesMachineID is the machine id the fixture's server state is opened with,
// so the seeded One-offs carry the Dedupe key the server builds.
const SchedulesMachineID = "fixture1"

// SchedulesGhost is the store folder of the missing-project shape: it has a
// Schedule but no project.json.
const SchedulesGhost = "ghost"

// ScheduleSeed is a Schedule written with the schedules store before the server
// starts. On turns its switch on here, which sets the baseline to the start time,
// so no catch-up run starts.
type ScheduleSeed struct {
	Project, Name, Prompt string
	Days                  []time.Weekday
	Times                 []string
	LateLimitHours        float64
	On                    bool
	Skips                 []schedules.Skip
}

// SchedulesShapes are the --shape names of the schedules scenario.
var SchedulesShapes = []string{"list", "all-off", "empty", "page", "paused", "missing-project"}

var (
	everyDay = []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday}
	weekdays = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
)

// SchedulesScenario is the schedules scenario in the given shape. Every set time
// is at least two hours ahead of now (local time), so the timer never fires
// during a flow.
func SchedulesScenario(shape string) (Scenario, bool) {
	now := time.Now()
	ahead := func(hours ...int) []string {
		out := make([]string, len(hours))
		for i, h := range hours {
			out[i] = now.Add(time.Duration(h) * time.Hour).Format("15:04")
		}
		return out
	}
	weather := ScheduleSeed{Project: project, Name: "Weather check", Prompt: "Check the weather for tomorrow.\nTell me if I need an umbrella.", Days: everyDay, Times: ahead(2, 5), LateLimitHours: 2, On: true}
	three := func(weatherOn, digestOn bool) []ScheduleSeed {
		w, d := weather, ScheduleSeed{Project: project, Name: "Morning PR digest", Prompt: "Summarise the pull requests opened yesterday.", Days: weekdays, Times: ahead(3), LateLimitHours: 2, On: digestOn}
		w.On = weatherOn
		return []ScheduleSeed{w, d, {Project: project, Name: "Weekly cleanup report", Prompt: "List the branches that can be deleted.", Days: []time.Weekday{time.Sunday}, Times: ahead(4), LateLimitHours: 2}}
	}
	sc := Scenario{Chat: true, Hold: true, Projects: []string{project}}
	switch shape {
	case "list":
		sc.Schedules = three(true, true)
	case "all-off":
		sc.Schedules = three(false, false)
	case "empty":
	case "page", "paused":
		sc.Schedules = []ScheduleSeed{weather}
		sc.Epics, sc.EpicMeta, sc.Schedules[0].Skips = weatherRuns(now, shape == "paused")
	case "missing-project":
		g := weather
		g.Project = SchedulesGhost
		sc.Schedules = []ScheduleSeed{g}
	default:
		return Scenario{}, false
	}
	return sc, true
}

// weatherRuns is the Last runs of the page shape, newest first: Finished, Finished,
// Ran late and a Skipped set time, with a paused One-off above them for paused.
func weatherRuns(now time.Time, paused bool) ([]Epic, map[string]string, []schedules.Skip) {
	key := schedules.DedupeKey(schedules.Key{Project: project, Slug: tickets.Slug("Weather check")}, SchedulesMachineID)
	day := 24 * time.Hour
	stamp := func(t time.Time) string { return t.Format(time.RFC3339) }
	run := func(due time.Time, lag time.Duration, result string) servertest.TicketOpts {
		return servertest.TicketOpts{
			Type: "prompt", Status: "done", Unique: key, Result: result,
			ScheduledFor: stamp(due), CreatedAt: stamp(due.Add(lag)), DoneAt: stamp(due.Add(lag + 10*time.Minute)),
		}
	}
	live := Epic{Project: project, Name: tickets.OneOffsEpic, Tickets: []Ticket{{"01", "weather-check", run(now.Add(-day), 5*time.Second, "Rain tomorrow, bring an umbrella.")}}}
	if paused {
		due := now.Add(-30 * time.Minute)
		live.Tickets = append(live.Tickets, Ticket{"02", "weather-check", servertest.TicketOpts{
			Type: "prompt", PauseReason: "question", Unique: key, ScheduledFor: stamp(due), CreatedAt: stamp(due.Add(5 * time.Second)),
		}})
	}
	archived := Epic{Project: project, Name: tickets.ArchivedOneOffsEpic, Tickets: []Ticket{
		{"01", "weather-check", run(now.Add(-2*day), 5*time.Second, "Clear skies, no umbrella.")},
		{"02", "weather-check", run(now.Add(-3*day), 5*time.Minute, "Light rain in the afternoon.")},
	}}
	meta := map[string]string{
		project + ":" + tickets.OneOffsEpic:         "status: draft\n",
		project + ":" + tickets.ArchivedOneOffsEpic: "status: draft\n",
	}
	missed := now.Add(-4 * day)
	skips := []schedules.Skip{{SetTime: missed, Reason: schedules.ReasonLate, LoggedAt: missed.Add(3 * time.Hour)}}
	return []Epic{live, archived}, meta, skips
}
