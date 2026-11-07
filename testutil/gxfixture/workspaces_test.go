package main

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

func TestWorkspacesScenario_SeedsWorkspacesHoldsAgentsAndACardPerSection(t *testing.T) {
	t.Parallel()
	r := start(t, "workspaces")
	// name|section|workspace. Queue agents appear once the server has launched the workers.
	want := []string{
		"ws-design|designing|gx bugs",
		"held-design|on-hold|gx bugs",
		"ws-impl-epic|implementing|research",
		"held-epic|on-hold|research",
		"one-off|implementing|",
		"paused-epic|needs-you|Default workspace",
		"ready-epic|ready-to-queue|Default workspace",
		"design-new|ready-to-design|Default workspace",
		"finished-a|finished|Default workspace",
	}
	var got []string
	var missing []string
	testutil.Poll(40*time.Second, 500*time.Millisecond, func() bool {
		got = got[:0]
		for _, c := range r.cardsOf(t) {
			got = append(got, c.Name+"|"+string(c.Section)+"|"+c.Workspace)
		}
		missing = slices.DeleteFunc(slices.Clone(want), func(w string) bool { return slices.Contains(got, w) })
		return len(missing) == 0
	})
	if len(missing) > 0 {
		t.Fatalf("missing cards %v\ngot:\n%s", missing, strings.Join(got, "\n"))
	}

	var list wire.WorkspaceList
	if err := json.Unmarshal([]byte(r.gx(t, "server", "workspaces", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	members := map[string]int{}
	for _, w := range list.Workspaces {
		members[w.Name] = w.Members
	}
	// Members are explicit memberships; the Default workspace holds what was never moved.
	if _, ok := members["Default workspace"]; !ok {
		t.Errorf("no Default workspace in %v", members)
	}
	for _, name := range []string{"gx bugs", "research"} {
		if members[name] == 0 {
			t.Errorf("workspace %q has %d members, want some; got %v", name, members[name], members)
		}
	}

	var sessions []wire.SessionInfo
	if !testutil.Poll(60*time.Second, time.Second, func() bool {
		sessions = nil
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &sessions); err != nil {
			t.Fatal(err)
		}
		return len(sessions) >= 3
	}) {
		t.Fatalf("agents list has %d sessions, want a chat, a Queue agent and a One-off agent", len(sessions))
	}
	r.stop(t)
}

func TestWorkspacesScenario_ServesClaudeLimitsAndTwoPausedEpicFixAgentsOneWorking(t *testing.T) {
	t.Parallel()
	r := start(t, "workspaces")
	defer r.stop(t)
	var sum wire.UsageSummary
	if err := json.Unmarshal([]byte(r.gx(t, "usage", "summary", "--json")), &sum); err != nil {
		t.Fatal(err)
	}
	if sum.Mode != wire.UsageModeLimits || sum.Limits.FiveHour == nil || sum.Limits.SevenDay == nil {
		t.Errorf("want both Claude limit windows, got mode %q: %+v", sum.Mode, sum.Limits)
	}
	// The Fix agents under paused-epic read as one Needs you (Idle) and one Working.
	want := map[string]string{
		"demo:paused-epic/01": wire.SessionIdle,
		"demo:paused-epic/02": wire.SessionWorking,
	}
	var got map[string]string
	if !testutil.Poll(60*time.Second, time.Second, func() bool {
		var sessions []wire.SessionInfo
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &sessions); err != nil {
			t.Fatal(err)
		}
		got = map[string]string{}
		for _, si := range sessions {
			if _, ok := want[si.FixOf]; ok {
				got[si.FixOf] = si.Status.State
			}
		}
		return maps.Equal(got, want)
	}) {
		t.Fatalf("fix agent states %v, want %v", got, want)
	}
}

func TestWorkspacesScenario_SeedsFixAgentsAndAnInvestigateAgentInOtherAndUnderEpics(t *testing.T) {
	t.Parallel()
	r := start(t, "workspaces")
	// fix_of -> placement kind and epic.
	wantFix := map[string]string{
		"demo":                "other|",
		"demo:one-offs/04":    "other|",
		"demo:paused-epic/01": "epic|demo:paused-epic",
	}
	var sessions []wire.SessionInfo
	var snap wire.Snapshot
	var problems []string
	testutil.Poll(60*time.Second, time.Second, func() bool {
		sessions, problems = nil, nil
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &sessions); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(r.gx(t, "server", "snapshot", "--json")), &snap); err != nil {
			t.Fatal(err)
		}
		placed := map[string]wire.SessionPlacement{}
		for _, p := range snap.Placements {
			placed[p.Session] = p
		}
		fixes := map[string]wire.SessionPlacement{}
		var investigate []wire.SessionPlacement
		for _, si := range sessions {
			if si.FixOf != "" {
				fixes[si.FixOf] = placed[si.ID]
			}
			if si.Queue && (strings.HasPrefix(si.Ticket, "demo:one-offs/01") || strings.HasPrefix(si.Ticket, "demo:inv-epic/02")) {
				investigate = append(investigate, placed[si.ID])
			}
		}
		for fixOf, want := range wantFix {
			if got := fixes[fixOf].Kind + "|" + fixes[fixOf].Epic; got != want {
				problems = append(problems, "fix agent of "+fixOf+": got "+got+", want "+want)
			}
		}
		if len(investigate) != 2 {
			problems = append(problems, "want two Investigate Queue agents")
		}
		return len(problems) == 0
	})
	if len(problems) > 0 {
		var rows []string
		for _, si := range sessions {
			rows = append(rows, si.ID+" ticket="+si.Ticket+" fix_of="+si.FixOf)
		}
		for _, tk := range snap.Tickets {
			rows = append(rows, "TICKET "+tk.Address+" "+tk.Status+" "+tk.RenderedStatus+" "+tk.Type+" parent="+tk.Parent+" step="+tk.LaunchStep.Label)
		}
		t.Fatalf("%s\nsessions:\n%s\nplacements: %+v", strings.Join(problems, "\n"), strings.Join(rows, "\n"), snap.Placements)
	}
	r.stop(t)
}
