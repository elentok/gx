package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

// appendMillionTokens adds one assistant line of a million input tokens to the held
// agent's transcript: $3.00 of spend and one turn for the long-ticket guard.
func appendMillionTokens(t *testing.T, r *running) { appendAssistantLine(t, r, 1_000_000) }

// appendAssistantLine adds one assistant turn with the given input tokens.
func appendAssistantLine(t *testing.T, r *running, inputTokens int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(r.AgentTranscript), 0o755); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":%d,"output_tokens":0}}}`+"\n",
		time.Now().Format(time.RFC3339), inputTokens)
	f, err := os.OpenFile(r.AgentTranscript, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
}

// projectRepo is the registered repo path of a project.
func projectRepo(t *testing.T, r *running, name string) string {
	t.Helper()
	var list []wire.ProjectInfo
	if err := json.Unmarshal([]byte(r.gx(t, "project", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.Name == name {
			return p.Repo
		}
	}
	t.Fatalf("no project %q in %+v", name, list)
	return ""
}

// setTrunk writes (or, empty, drops) the trunk key of a project's project.json.
func setTrunk(t *testing.T, r *running, project, trunk string) {
	t.Helper()
	// The capture file sits at the fixture root, beside the ticket store.
	path := filepath.Join(filepath.Dir(r.ChatCapture), "data", "gx", "tickets", project, "project.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pf := map[string]any{}
	if err := json.Unmarshal(data, &pf); err != nil {
		t.Fatal(err)
	}
	if trunk == "" {
		delete(pf, "trunk")
	} else {
		pf["trunk"] = trunk
	}
	out, err := json.Marshal(pf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// The producible notices are checked in three tests, each on its own server,
// so each stays inside the per-test time limit.

func TestChatServerScenario_StartAndDiskNoticesLandInTheCaptureFile(t *testing.T) {
	t.Parallel()
	r := start(t, "chat-server")

	waitForParts(t, r, map[string][]string{"server-started": {"Server started"}})

	// Low disk: the hold starts a clean on its own, and the idle clean script
	// frees nothing. Space returning on an ordinary reading resumes dispatch.
	r.setDisk(t, "free", "5G")
	waitForParts(t, r, map[string][]string{
		"disk-low":   {"Low disk space"},
		"disk-clean": {"Disk clean finished"},
	})
	r.setDisk(t, "free", "25G")
	waitForParts(t, r, map[string][]string{"disk-resume": {"Dispatch resumed"}})
}

func TestChatServerScenario_BudgetAndLongTicketNoticesLandInTheCaptureFile(t *testing.T) {
	t.Parallel()
	r := start(t, "chat-server")

	// The long-ticket guard counts only turns after its launch started, so free
	// turns are appended until it fires; the hard limit would stop the agent, so
	// this comes before any spend.
	if !testutil.Poll(60*time.Second, 250*time.Millisecond, func() bool {
		appendAssistantLine(t, r, 0)
		for _, l := range captured(r.ChatCapture) {
			if l.Transport == "slack" && strings.Contains(l.Text, "Long ticket") {
				return true
			}
		}
		return false
	}) {
		t.Fatal("no long ticket notice")
	}

	// Spend: the ledger starts under the soft limit; each line adds $3.
	appendMillionTokens(t, r)
	waitForParts(t, r, map[string][]string{"budget-soft": {"Budget soft limit"}})
	appendMillionTokens(t, r)
	waitForParts(t, r, map[string][]string{"budget-hard": {"Budget hard limit"}})
}

func TestChatServerScenario_ProjectAndTrunkNoticesLandInTheCaptureFile(t *testing.T) {
	t.Parallel()
	r := start(t, "chat-server")

	// A project whose path is gone.
	if err := os.RemoveAll(projectRepo(t, r, "gone")); err != nil {
		t.Fatal(err)
	}
	waitForParts(t, r, map[string][]string{"project-unavailable": {"gone", "unavailable"}})

	// A trunk that goes missing and returns.
	setTrunk(t, r, "demo", "no-such-trunk")
	waitForParts(t, r, map[string][]string{"trunk-missing": {"trunk missing"}})
	setTrunk(t, r, "demo", "")
	waitForParts(t, r, map[string][]string{"trunk-found": {"trunk found"}})
}
