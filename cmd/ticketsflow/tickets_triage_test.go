package ticketsflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
)

// triageProject is a project with two open follow-ups.
func triageProject(t *testing.T) (repo string) {
	t.Helper()
	repo, project := setupShowProject(t)
	for _, id := range []string{"01", "02"} {
		writeEpicTicket(t, project, tickets.FollowUpsEpic, id+"-f.md",
			"---\nid: \""+id+"\"\nstatus: open\ntype: research\ncategory: bug\n---\n\n# Follow-up "+id+"\n")
	}
	return repo
}

type triageShown struct {
	ID       string `json:"id"`
	Step     string `json:"step"`
	Next     string `json:"next"`
	Revision int    `json:"revision"`
	Rows     []struct {
		Address string `json:"address"`
		Verdict string `json:"verdict"`
		Write   string `json:"write"`
	} `json:"rows"`
}

func triageShow(t *testing.T, repo string) triageShown {
	t.Helper()
	out, err := runIn(t, repo, "tickets", "triage", "show", "--json")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	var s triageShown
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("show json %q: %v", out, err)
	}
	return s
}

func triagePending(t *testing.T, repo string) []string {
	t.Helper()
	out, err := runIn(t, repo, "tickets", "triage", "pending", "--json")
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	var rows []struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("pending json %q: %v", out, err)
	}
	var addrs []string
	for _, r := range rows {
		addrs = append(addrs, r.Address)
	}
	return addrs
}

func TestTriage_CreateThenSecondCreateRefusesNamingLive(t *testing.T) {
	repo := triageProject(t)
	out, err := runIn(t, repo, "tickets", "triage", "create")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := strings.TrimSpace(out)
	if id == "" {
		t.Fatal("create printed no id")
	}
	_, err = runIn(t, repo, "tickets", "triage", "create")
	if err == nil || !strings.Contains(err.Error(), id) {
		t.Errorf("second create = %v, want a refusal naming %s", err, id)
	}
}

func TestTriage_ShowNextAcrossTheVerifyStep(t *testing.T) {
	repo := triageProject(t)
	if s := triageShow(t, repo); s.Next != "start" {
		t.Errorf("empty next = %q, want start", s.Next)
	}
	if _, err := runIn(t, repo, "tickets", "triage", "create"); err != nil {
		t.Fatal(err)
	}
	s := triageShow(t, repo)
	if s.Next != "verify" || len(s.Rows) != 2 {
		t.Errorf("pending next/rows = %q/%d, want verify/2", s.Next, len(s.Rows))
	}
	for _, addr := range triagePending(t, repo) {
		if _, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", addr, "--evidence", "e"); err != nil {
			t.Fatalf("verdict %s: %v", addr, err)
		}
	}
	if s := triageShow(t, repo); s.Next != "plan" || s.Step != "plan" {
		t.Errorf("after last verdict next/step = %q/%q, want plan/plan", s.Next, s.Step)
	}
}

func TestTriage_PendingShrinksAndGoneIsSaved(t *testing.T) {
	repo := triageProject(t)
	if _, err := runIn(t, repo, "tickets", "triage", "create"); err != nil {
		t.Fatal(err)
	}
	pending := triagePending(t, repo)
	if len(pending) != 2 {
		t.Fatalf("pending = %v, want 2", pending)
	}
	if _, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", pending[0], "--verdict", "fixed"); err != nil {
		t.Fatal(err)
	}
	// A second session reads only the rows with no verdict.
	rest := triagePending(t, repo)
	if len(rest) != 1 || rest[0] != pending[1] {
		t.Fatalf("pending after one verdict = %v, want [%s]", rest, pending[1])
	}
	if _, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", pending[1], "--gone"); err != nil {
		t.Fatal(err)
	}
	if got := triagePending(t, repo); len(got) != 0 {
		t.Errorf("pending = %v, want none", got)
	}
	s := triageShow(t, repo)
	for _, r := range s.Rows {
		if r.Address == pending[1] && r.Write != "gone" {
			t.Errorf("gone row write = %q, want gone", r.Write)
		}
	}
}

func TestTriage_StaleRevisionIsRefusedAndRecordUnchanged(t *testing.T) {
	repo := triageProject(t)
	if _, err := runIn(t, repo, "tickets", "triage", "create"); err != nil {
		t.Fatal(err)
	}
	before := triageShow(t, repo)
	row := before.Rows[0].Address
	_, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", row, "--revision", "99")
	if err == nil {
		t.Fatal("stale revision accepted")
	}
	after := triageShow(t, repo)
	if after.Revision != before.Revision || after.Rows[0].Verdict != "" {
		t.Errorf("record changed: revision %d -> %d, verdict %q", before.Revision, after.Revision, after.Rows[0].Verdict)
	}
	out, err := runIn(t, repo, "tickets", "triage", "verdict", "--row", row, "--revision", "1")
	if err != nil {
		t.Fatalf("current revision refused: %v", err)
	}
	if strings.TrimSpace(out) != "2" {
		t.Errorf("printed revision %q, want 2", out)
	}
}
