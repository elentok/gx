package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func TestHerdrConfigWarnsOnceAndRunsOnPty(t *testing.T) {
	r := start(t, "herdr-config")
	var st struct {
		Running bool `json:"running"`
	}
	if err := json.Unmarshal([]byte(r.Gx(t, "server", "status", "--json")), &st); err != nil || !st.Running {
		t.Fatalf("server status = %+v, %v; want running", st, err)
	}
	const warning = "agent-runner: herdr was removed; using auto (PTY). Edit the config to drop it."
	if n := strings.Count(r.Gx(t, "server", "logs"), warning); n != 1 {
		t.Errorf("server log holds the warning %d times, want 1", n)
	}
	r.WaitStatus(t, project+":herdr-config-epic/01", "claimed")
}

func TestHerdrLeftoverEmptyReopensOnPty(t *testing.T) {
	r := start(t, "herdr-leftover-empty")
	reopened := false
	if !testutil.Poll(60*time.Second, time.Second, func() bool {
		reopened = strings.Contains(r.Gx(t, "server", "tickets", "history", spec.HerdrLeftoverEmptyTicket), "claim-reopened")
		return reopened || strings.Contains(r.epicTicketFile(t, "herdr-empty-epic"), "pause_reason:")
	}) {
		t.Fatal("the ticket was neither reopened nor paused")
	}
	if !reopened {
		t.Fatalf("the start did not reopen the ticket:\n%s", r.epicTicketFile(t, "herdr-empty-epic"))
	}
	r.WaitStatus(t, spec.HerdrLeftoverEmptyTicket, "claimed")
}

func TestHerdrLeftoverWorkPausesHandleMismatch(t *testing.T) {
	r := start(t, "herdr-leftover-work")
	r.WaitStatus(t, spec.HerdrLeftoverWorkTicket, "needs-repair")
	body := r.epicTicketFile(t, "herdr-work-epic")
	if !strings.Contains(body, "pause_reason: handle-mismatch") {
		t.Fatalf("ticket is not paused handle-mismatch:\n%s", body)
	}
	paused := r.Gx(t, "tickets", "paused", spec.HerdrLeftoverWorkTicket)
	for _, want := range []string{"herdr runner was removed", "Retry or Start over ends this pause"} {
		if !strings.Contains(body+paused, want) {
			t.Errorf("the pause does not say %q:\n%s", want, body)
		}
	}
}

func TestHerdrAgentPaneBusyMigratesToIterationError(t *testing.T) {
	r := start(t, "agent-pane-busy")
	body := r.epicTicketFile(t, "pane-busy-epic")
	if !strings.Contains(body, "pause_reason: iteration-error") || strings.Contains(body, "agent_pane_busy") {
		t.Errorf("ticket was not migrated to iteration-error:\n%s", body)
	}
}

func TestHerdrOldRunLogReads(t *testing.T) {
	r := start(t, "old-run-log")
	log := filepath.Join(r.StateDir, "..", "..", "data", "gx", "tickets", project, "old-log-epic", "run-log.jsonl")
	if b, err := os.ReadFile(log); err != nil || !strings.Contains(string(b), "herdr-agent-lost") {
		t.Fatalf("seeded run log = %q, %v; want the old events", b, err)
	}
	r.Gx(t, "server", "tickets", "history", project+":old-log-epic/01")
	if code := r.Get(t, r.WebURL+"/v1/snapshot", true); code != http.StatusOK {
		t.Errorf("GET /v1/snapshot = %d, want 200", code)
	}
}
