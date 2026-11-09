package designflows

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

// The read-only TestWorkspacesScenario_* tests share one workspaces fixture,
// which TestMain starts and stops: under a whole-package run, a fixture per
// test loaded the machine enough to push them past their time limit.
var workspaces *running

func sharedWorkspaces(t *testing.T) *running {
	t.Helper()
	if workspaces == nil {
		t.Skip("the workspaces fixture is not running: -run skipped the Workspaces tests")
	}
	return workspaces
}

func TestWorkspacesScenario_SeedsWorkspacesHoldsAgentsAndACardPerSection(t *testing.T) {
	t.Parallel()
	r := sharedWorkspaces(t)
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
		"follow-ups|ready-to-design|", // a project card, in no workspace
		"finished-a|finished|Default workspace",
		"side:side-design|ready-to-design|Default workspace", // the second project's Design
	}
	var got []string
	var missing []string
	testutil.Poll(40*time.Second, 500*time.Millisecond, func() bool {
		got = got[:0]
		for _, c := range r.cardsOf(t) {
			name := c.Name
			if c.Project != "demo" {
				name = c.Project + ":" + c.Name
			}
			got = append(got, name+"|"+string(c.Section)+"|"+c.Workspace)
		}
		missing = slices.DeleteFunc(slices.Clone(want), func(w string) bool { return slices.Contains(got, w) })
		return len(missing) == 0
	})
	if len(missing) > 0 {
		t.Fatalf("missing cards %v\ngot:\n%s", missing, strings.Join(got, "\n"))
	}

	var list wire.WorkspaceList
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "workspaces", "list", "--json")), &list); err != nil {
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
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &sessions); err != nil {
			t.Fatal(err)
		}
		// The second project's chat, for New chat's project list.
		return len(sessions) >= 3 && slices.ContainsFunc(sessions, func(si wire.SessionInfo) bool {
			return si.Project == "side" && !si.Queue && si.FixOf == ""
		})
	}) {
		t.Fatalf("agents list has %d sessions, want a chat, a Queue agent, a One-off agent and a chat in project side", len(sessions))
	}
}

func TestWorkspacesScenario_ServesClaudeLimitsAndTwoPausedEpicFixAgentsOneWorking(t *testing.T) {
	t.Parallel()
	r := sharedWorkspaces(t)
	var sum wire.UsageSummary
	if err := json.Unmarshal([]byte(r.Gx(t, "usage", "summary", "--json")), &sum); err != nil {
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
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &sessions); err != nil {
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
	r := sharedWorkspaces(t)
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
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &sessions); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(r.Gx(t, "server", "snapshot", "--json")), &snap); err != nil {
			t.Fatal(err)
		}
		placed := map[string]wire.SessionPlacement{}
		for _, p := range snap.Placements {
			placed[p.Session] = p
		}
		fixes := map[string]wire.SessionPlacement{}
		var investigate []wire.SessionPlacement
		var nested *wire.SessionPlacement
		for _, si := range sessions {
			if si.FixOf != "" {
				fixes[si.FixOf] = placed[si.ID]
			}
			if si.Queue && (strings.HasPrefix(si.Ticket, "demo:one-offs/01") || strings.HasPrefix(si.Ticket, "demo:inv-epic/02")) {
				investigate = append(investigate, placed[si.ID])
			}
			if si.Queue && strings.HasPrefix(si.Ticket, "demo:inv-epic/02") {
				p := placed[si.ID]
				nested = &p
			}
		}
		if nested == nil || nested.Kind != "epic" || nested.Epic != "demo:inv-epic" {
			problems = append(problems, "want the inv-epic/02 Queue agent nested under demo:inv-epic")
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
}
