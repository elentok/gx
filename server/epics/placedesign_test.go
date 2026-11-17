package epics

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func TestPlaceDesign_Working(t *testing.T) {
	t0 := time.Unix(100, 0)
	cases := []struct {
		name  string
		act   designActivity
		step  wire.ProgressStep
		words string
		work  bool
	}{
		{"reported step wins", designActivity{running: true, working: true, reportedStep: "grill-findings"}, wire.ProgressSpec, "Reviewing", true},
		{"unknown report falls to map step", designActivity{running: true, working: true, reportedStep: "zzz"}, wire.ProgressResolveTickets, "Researching", true},
		{"subagent only falls to map step", designActivity{running: true, working: true, delegating: true}, wire.ProgressBranch, "Branching", true},
		{"done falls to Designing", designActivity{running: true, working: true}, wire.ProgressDone, "Designing", true},
		{"idle session does not work", designActivity{running: true}, wire.ProgressSpec, "Designing", false},
		{"no session does not work", designActivity{}, wire.ProgressCreateTickets, "Designing", false},
		{"creating works while idle", designActivity{running: true, creating: true}, wire.ProgressCreateTickets, "Creating tickets", true},
		{"creating works in a countdown", designActivity{running: true, creating: true, countdownAt: t0}, wire.ProgressCreateTickets, "Creating tickets", true},
		{"countdown otherwise keeps words", designActivity{running: true, working: true, countdownAt: t0}, wire.ProgressSpec, "Designing", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := placeDesign("proj:map", c.act, false, time.Time{}, c.step)
			if p.working != c.work {
				t.Fatalf("working = %v, want %v", p.working, c.work)
			}
			if c.work {
				if p.workingWord != c.words || p.statusWords != c.words+"..." {
					t.Fatalf("word %q words %q, want %q", p.workingWord, p.statusWords, c.words)
				}
			} else if p.statusWords != c.words || p.workingWord != "" {
				t.Fatalf("words %q word %q", p.statusWords, p.workingWord)
			}
		})
	}
}

func TestPlaceDesign_QuestionBeatsWorking(t *testing.T) {
	p := placeDesign("proj:map", designActivity{running: true, working: true, asking: true}, false, time.Time{}, wire.ProgressSpec)
	if p.section != wire.SectionNeedsYou || p.working {
		t.Fatalf("section %q working %v", p.section, p.working)
	}
}

func TestDecorateLive_MapWorkingFlag(t *testing.T) {
	root := queue.RootRef{Project: "p", Epic: "map"}
	e := tickets.Epic{Name: "map", IsMap: true}
	for _, c := range []struct {
		name string
		info wire.SessionInfo
		want bool
	}{
		{"turn", wire.SessionInfo{Status: wire.SessionStatus{State: wire.SessionWorking}}, true},
		{"starting", wire.SessionInfo{Status: wire.SessionStatus{State: wire.SessionStarting}}, true},
		{"subagent", wire.SessionInfo{Status: wire.SessionStatus{State: wire.SessionWorking, Background: true}}, true},
		{"idle", wire.SessionInfo{Status: wire.SessionStatus{State: wire.SessionIdle}}, false},
		{"exited", wire.SessionInfo{Status: wire.SessionStatus{State: wire.SessionExited}}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.info.Design, c.info.Ticket = true, "p:map/01"
			deps := fakeEpics()
			deps.Sessions = fakeSessions{list: []wire.SessionInfo{c.info}}
			deps.DesignUncharted = func(string, tickets.Epic) (bool, error) { return false, nil }
			d := wire.EpicDetail{Address: root.String()}
			New(deps).decorateLive(&d, root, e)
			if d.Working != c.want {
				t.Fatalf("Working = %v, want %v", d.Working, c.want)
			}
		})
	}
}

func TestDecorateDesignCard_CreateTickets(t *testing.T) {
	m := tickets.Epic{Name: "map", IsMap: true}
	impl := &wire.EpicListRow{Address: "proj:map-impl", Total: 4}
	var row wire.EpicListRow
	row.Address, row.ProgressStep = "proj:map", wire.ProgressCreateTickets
	decorateDesignCard(&row, m, impl, designActivity{running: true})
	if !row.Working || row.WorkingWord != "Creating tickets" || row.WorkKind != wire.WorkKindDesign ||
		row.StatusWords != "Creating tickets (4 so far)..." || row.RingProgress != 0 {
		t.Fatalf("idle session: %+v", row)
	}

	row = wire.EpicListRow{Address: "proj:map", ProgressStep: wire.ProgressCreateTickets}
	decorateDesignCard(&row, m, impl, designActivity{})
	if row.Working || row.StatusWords != "Designing" || row.Reason != "No design session is running" ||
		strings.Contains(row.StatusWords, "so far") {
		t.Fatalf("no session: %+v", row)
	}
}
