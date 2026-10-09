package server

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func investigateReport(class string) string {
	return "\n# 02 — Investigate 01\n\n## Result\n\n### Observed\n\nIt spun.\n\n### Evidence\n\nlog line\n\n### Diagnosis\n\nA loop.\n\n### Proposal\n\n" + class + ": add a rule\n"
}

// landedInvestigation writes an investigate ticket carrying body into a fresh
// store and returns the server, the store and the ticket's address and path.
func landedInvestigation(t *testing.T, body string, followUps string) (*Server, string, tickets.Address, string) {
	t.Helper()
	store := t.TempDir()
	epicPath := filepath.Join(store, "proj", "epic-a")
	if err := os.MkdirAll(filepath.Join(epicPath, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, epicPath)
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(`{"repo":"/x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	parent := schema.TicketID("01")
	out, err := schema.MarshalTicket(schema.Ticket{ID: "02", Status: schema.StatusDone, Type: schema.TypeInvestigate, Parent: &parent}, body)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(epicPath, "issues", "02-investigate.md")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: Config{TicketStore: store, RecoverySettings: config.RecoveryConfig{FollowUps: followUps}}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return s, store, tickets.Address{Project: "proj", Epic: "epic-a", ID: "02"}, path
}

func TestFileFollowUp_CatalogEntryFilesADraftResearchTicketAndCreatesTheEpicDraft(t *testing.T) {
	for _, class := range []string{proposalCatalogEntry, proposalOrchestratorFix} {
		t.Run(class, func(t *testing.T) {
			s, store, addr, path := landedInvestigation(t, investigateReport(class), "proj:follow-ups")
			s.fileFollowUp(addr, path)

			epicPath := filepath.Join(store, "proj", "follow-ups")
			epics, err := tickets.Load(filepath.Join(store, "proj"))
			if err != nil {
				t.Fatal(err)
			}
			var got *tickets.Epic
			for i := range epics {
				if epics[i].Name == "follow-ups" {
					got = &epics[i]
				}
			}
			if got == nil || got.Status != string(schema.StatusDraft) || len(got.Tickets) != 1 {
				t.Fatalf("follow-ups epic = %+v, want a draft epic with one ticket", got)
			}
			tk := got.Tickets[0]
			if tk.Type != string(schema.TypeResearch) || tk.Status != string(schema.StatusDraft) {
				t.Errorf("ticket = %s/%s, want draft research", tk.Status, tk.Type)
			}
			if _, err := os.Stat(epicPath); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestEnsureDraftEpic_ExistingDirWithoutTicketMDBecomesValid(t *testing.T) {
	projectDir := t.TempDir()
	epicPath := filepath.Join(projectDir, "follow-ups")
	testutil.Mkdir(t, epicPath)

	if err := ensureDraftEpic(epicPath); err != nil {
		t.Fatal(err)
	}

	if err := tickets.ValidateProject(projectDir); err != nil {
		t.Errorf("ValidateProject: %v", err)
	}
	epics, err := tickets.Load(projectDir)
	if err != nil || len(epics) != 1 || epics[0].Status != string(schema.StatusDraft) {
		t.Errorf("Load = %+v, %v; want one draft epic", epics, err)
	}
}

func followUpTickets(t *testing.T, store string) []tickets.Ticket {
	t.Helper()
	epics, err := tickets.Load(filepath.Join(store, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range epics {
		if e.Name == "follow-ups" {
			return e.Tickets
		}
	}
	return nil
}

func TestFileFollowUp_SecondIdenticalReportAddsAnOccurrenceLine(t *testing.T) {
	s, store, addr, path := landedInvestigation(t, investigateReport(proposalCatalogEntry), "proj:follow-ups")
	s.fileFollowUp(addr, path)
	s.fileFollowUp(addr, path)

	got := followUpTickets(t, store)
	if len(got) != 1 {
		t.Fatalf("follow-ups has %d tickets, want 1", len(got))
	}
	raw, err := os.ReadFile(got[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "## Comments") || !strings.Contains(string(raw), "Seen again in proj:epic-a/02") {
		t.Errorf("ticket has no occurrence line:\n%s", raw)
	}
}

func TestFileFollowUp_DifferentClassIsNotADuplicate(t *testing.T) {
	s, store, addr, path := landedInvestigation(t, investigateReport(proposalCatalogEntry), "proj:follow-ups")
	s.fileFollowUp(addr, path)
	_, _, _, other := landedInvestigation(t, investigateReport(proposalOrchestratorFix), "proj:follow-ups")
	raw, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	s.fileFollowUp(addr, path)

	if got := followUpTickets(t, store); len(got) != 2 {
		t.Errorf("follow-ups has %d tickets, want 2", len(got))
	}
}

func TestFileFollowUp_MissingProjectFilesNothingAndDoesNotFail(t *testing.T) {
	s, store, addr, path := landedInvestigation(t, investigateReport(proposalCatalogEntry), "nowhere:follow-ups")
	s.fileFollowUp(addr, path)
	if got := followUpTickets(t, store); got != nil {
		t.Errorf("filed %d tickets with no follow-ups project", len(got))
	}
}

func TestFileFollowUp_HumanOnlyFilesNothing(t *testing.T) {
	s, store, addr, path := landedInvestigation(t, investigateReport(proposalHumanOnly), "proj:follow-ups")
	s.fileFollowUp(addr, path)
	if _, err := os.Stat(filepath.Join(store, "proj", "follow-ups")); !os.IsNotExist(err) {
		t.Errorf("follow-ups epic exists after a human-only report: %v", err)
	}
}

func TestProposalClass_AmbiguousOrMissingFilesNothing(t *testing.T) {
	for name, result := range map[string]string{
		"no proposal":  "Observed: it spun",
		"two classes":  "Proposal: catalog-entry or orchestrator-fix",
		"no class":     "Proposal: do something",
		"class before": "catalog-entry\nProposal: human-only",
	} {
		t.Run(name, func(t *testing.T) {
			want := ""
			if name == "class before" {
				want = proposalHumanOnly
			}
			if got := proposalClass(result); got != want {
				t.Errorf("proposalClass = %q, want %q", got, want)
			}
		})
	}
}
