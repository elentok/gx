package main

import (
	"encoding/json"
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
