package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
)

func TestFakeHarness_OnlyInTheGxfixtureBuild(t *testing.T) {
	t.Parallel()
	goFiles := func(args ...string) string {
		out, err := exec.Command("go", append([]string{"list", "-f", "{{.GoFiles}}"}, append(args, "github.com/elentok/gx")...)...).Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	if got := goFiles(); strings.Contains(got, "fixture_harness.go") {
		t.Errorf("plain build files = %s, want no fixture_harness.go", got)
	}
	if got := goFiles("-tags", "gxfixture"); !strings.Contains(got, "fixture_harness.go") {
		t.Errorf("gxfixture build files = %s, want fixture_harness.go", got)
	}
}

func TestFakeHarness_LiveIdleSessionWithAScreen(t *testing.T) {
	t.Parallel()
	r := start(t, "united")
	repo := filepath.Join(filepath.Dir(filepath.Dir(r.StateDir)), "repo")
	var res server.SessionResult
	out := r.gx(t, "agents", "create", "--harness", fakeHarness, "--project", "demo", "--dir", repo, "--json")
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Session == nil {
		t.Fatalf("agents create = %s, %v", out, err)
	}
	id := res.Session.ID
	r.waitIdle(t, id)

	var list []server.SessionInfo
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range list {
		if s.ID == id {
			found = true
			if s.Harness != fakeHarness || s.Status.State != server.SessionIdle {
				t.Errorf("session = harness %q state %q, want %s idle", s.Harness, s.Status.State, fakeHarness)
			}
		}
	}
	if !found {
		t.Fatalf("session %s not in list", id)
	}

	// The program draws nothing visible, so the rows may be blank; each is a line.
	if screen := r.gx(t, "agents", "screen", id); !strings.Contains(screen, "\n") {
		t.Errorf("screen = %q, want rows", screen)
	}
}
