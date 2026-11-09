package webflows

import (
	"encoding/json"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/spec"

	"github.com/elentok/gx/transcript"
)

func TestDefaultHarnessScenario_SessionsProjectsAndAWaitingTicket(t *testing.T) {
	t.Parallel()
	r := startWith(t, "default-harness", nil, "--fakecodex", fixturetest.FakeCodexBin)
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	harnesses := map[string]int{}
	for _, s := range list {
		if s.Design || s.Status.State == wire.SessionExited {
			continue
		}
		harnesses[s.Harness]++
	}
	if harnesses["codex"] != 1 || harnesses["claude"] < 2 {
		t.Errorf("session harnesses = %v, want one codex and at least two claude", harnesses)
	}

	// No wire field names the switch yet, so read the model the replies report.
	var models []string
	paths, _ := filepath.Glob(filepath.Join(r.Root(), "home", ".claude", "projects", "*", "*.jsonl"))
	for _, p := range paths {
		if u, ok, err := transcript.LastAssistantUsage(p); err == nil && ok {
			models = append(models, u.Model)
		}
	}
	if !slices.Contains(models, spec.SwitchedModel) {
		t.Errorf("newest reply models = %v, want one session on %s", models, spec.SwitchedModel)
	}

	overrides := 0
	for _, name := range []string{"demo", "dotfiles", "gx"} {
		body, err := os.ReadFile(filepath.Join(r.Root(), "data", "gx", "tickets", name, "project.json"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), `"default-harness":"codex"`) {
			overrides++
			if name != "gx" {
				t.Errorf("%s overrides the harness, want only gx", name)
			}
		}
	}
	if overrides != 1 {
		t.Errorf("%d project overrides, want 1", overrides)
	}

	if got := r.TicketStatus(t, "demo:waiting-epic/01"); got != "open" {
		t.Errorf("waiting ticket status = %q, want open", got)
	}
}

func TestFakeCodexFlag_InstallsCodexOnTheFixturePath(t *testing.T) {
	t.Parallel()
	with := startWith(t, "default-harness", nil, "--fakecodex", fixturetest.FakeCodexBin)
	if _, err := os.Stat(filepath.Join(with.Root(), "bin", "codex")); err != nil {
		t.Errorf("with --fakecodex: %v", err)
	}
	without := start(t, "empty")
	if _, err := os.Stat(filepath.Join(without.Root(), "bin", "codex")); !os.IsNotExist(err) {
		t.Errorf("without --fakecodex: codex stat = %v, want not exist", err)
	}
	if _, err := exec.LookPath(filepath.Join(with.Root(), "bin", "codex")); err != nil {
		t.Errorf("codex not runnable: %v", err)
	}
}
