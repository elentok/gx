package cmd

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
)

func TestServerEventsKinds_TextListsEveryKind(t *testing.T) {
	var out bytes.Buffer
	if err := runServerEventsKinds(false, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != len(events.Kinds()) {
		t.Fatalf("got %d lines, want %d", len(lines), len(events.Kinds()))
	}
	for _, k := range events.Kinds() {
		if !strings.Contains(out.String(), string(k)) {
			t.Errorf("kind %q missing from output", k)
		}
	}
	if !strings.Contains(out.String(), "agent_name_taken\tcause: herdr") {
		t.Errorf("herdr attribute missing:\n%s", out.String())
	}
}

func TestServerEventsKinds_JSONCarriesHerdrCause(t *testing.T) {
	var out bytes.Buffer
	if err := runServerEventsKinds(true, &out); err != nil {
		t.Fatal(err)
	}
	var got []KindInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(events.Kinds()) {
		t.Fatalf("got %d kinds, want %d", len(got), len(events.Kinds()))
	}
	for _, i := range got {
		if want := events.Kind(i.Kind).CauseHerdr(); i.CauseHerdr != want {
			t.Errorf("%s: cause_herdr=%v, want %v", i.Kind, i.CauseHerdr, want)
		}
	}
}

func TestServerEventsKinds_ViaRootCommand(t *testing.T) {
	root := newRootCmd(deps{})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"server", "events", "kinds", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) {
		t.Errorf("not JSON: %s", out.String())
	}
}

func TestStatusWarnings(t *testing.T) {
	remote := t.TempDir()
	if err := exec.Command("git", "init", "-q", remote).Run(); err != nil {
		t.Fatal(err)
	}
	noRemote := statusWarnings(apiclient.Negotiation{}, remote, "")
	if len(noRemote) != 1 || !strings.Contains(noRemote[0], "no push remote") {
		t.Fatalf("no-remote warnings = %q", noRemote)
	}
	if err := exec.Command("git", "-C", remote, "remote", "add", "origin", "x:y").Run(); err != nil {
		t.Fatal(err)
	}
	if got := statusWarnings(apiclient.Negotiation{}, remote, ""); len(got) != 0 {
		t.Fatalf("clean warnings = %q", got)
	}
	n := apiclient.Negotiation{Handshake: server.Handshake{TCPAddr: "127.0.0.1:1"}, Hint: "server is an older build"}
	all := strings.Join(statusWarnings(n, remote, ""), "\n")
	for _, want := range []string{"TCP listener is on", "older build"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in %q", want, all)
		}
	}
}

// The server reads orchestrator only at start, so a config change after that
// leaves it running the old value until a restart.
func TestStatusWarnings_OrchestratorMismatch(t *testing.T) {
	remote := t.TempDir()
	if err := exec.Command("git", "init", "-q", remote).Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", remote, "remote", "add", "origin", "x:y").Run(); err != nil {
		t.Fatal(err)
	}
	n := apiclient.Negotiation{Handshake: server.Handshake{Orchestrator: "in-process"}}
	got := strings.Join(statusWarnings(n, remote, "server"), "\n")
	for _, want := range []string{`"in-process"`, `"server"`, "gx server restart"} {
		if !strings.Contains(got, want) {
			t.Errorf("warnings = %q, want %q", got, want)
		}
	}
	n.Orchestrator = "server"
	if got := statusWarnings(n, remote, "server"); len(got) != 0 {
		t.Errorf("matching orchestrator warnings = %q, want none", got)
	}
}

func TestWaitForExit_WaitsUntilPidIsGone(t *testing.T) {
	checks := 0
	alive := func(int) bool { checks++; return checks < 3 }
	if err := waitForExit(42, time.Now().Add(time.Second), alive); err != nil {
		t.Fatal(err)
	}
	if checks != 3 {
		t.Fatalf("got %d alive checks, want 3", checks)
	}
}

func TestWaitForExit_PidOutlivesDeadline(t *testing.T) {
	alive := func(int) bool { return true }
	err := waitForExit(42, time.Now().Add(100*time.Millisecond), alive)
	if err == nil || !strings.Contains(err.Error(), "pid 42") {
		t.Fatalf("got %v, want a still-running error", err)
	}
}
