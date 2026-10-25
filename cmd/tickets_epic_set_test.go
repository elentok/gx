package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
)

const epicTicketMD = "---\nkind: loop\nstatus: draft\nstarted_at: 2026-01-02T03:04:05Z\ncustom: keep-me\n---\n\n# Epic\n\nbody words\n"

func setupEpicSetProject(t *testing.T) (d deps, ticketMD string) {
	t.Helper()
	cwd, project := setupEpicAddProject(t)
	epicDir := filepath.Join(project, "e")
	if err := os.MkdirAll(filepath.Join(epicDir, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	ticketMD = filepath.Join(epicDir, "ticket.md")
	if err := os.WriteFile(ticketMD, []byte(epicTicketMD), 0o644); err != nil {
		t.Fatal(err)
	}
	return deps{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, getwd: func() (string, error) { return cwd, nil }}, ticketMD
}

func TestTicketsSet_EpicAddressWritesSettableFieldsAndKeepsTheRest(t *testing.T) {
	d, ticketMD := setupEpicSetProject(t)

	err := execute([]string{"tickets", "set", "mine:e", "--kind=map", "--status=open", "--destination-unconfirmed=true"}, d)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	raw, _ := os.ReadFile(ticketMD)
	for _, want := range []string{"kind: map", "status: open", "destination_unconfirmed: true", "custom: keep-me", "started_at: 2026-01-02T03:04:05Z", "body words"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("ticket.md missing %q:\n%s", want, raw)
		}
	}
}

func TestTicketsSet_EpicAddressRejectsRemovedProvisionalDestinationFlag(t *testing.T) {
	d, ticketMD := setupEpicSetProject(t)

	if err := execute([]string{"tickets", "set", "mine:e", "--provisional-destination=true"}, d); err == nil {
		t.Fatal("set --provisional-destination succeeded, want unknown-flag error")
	}
	if raw, _ := os.ReadFile(ticketMD); string(raw) != epicTicketMD {
		t.Errorf("ticket.md changed:\n%s", raw)
	}
}

func TestTicketsSet_EpicAddressRefusesServerStampedFields(t *testing.T) {
	for _, flag := range []string{"--completed-at=2026-01-01T00:00:00Z", "--merged-at=2026-01-01T00:00:00Z", "--merged-at=", "--queued-at=2026-01-01T00:00:00Z", "--evolved-at=2026-01-01T00:00:00Z", "--evolve-skipped=true", "--evolve-ticket=mine:e/01", "--lines-added=1", "--lines-removed=1", "--total-tokens=1", "--actual-cost=1", "--had-codex=true", "--friction=x"} {
		d, ticketMD := setupEpicSetProject(t)
		err := execute([]string{"tickets", "set", "mine:e", "--status=open", flag}, d)
		var addrErr *tickets.AddressError
		if !errors.As(err, &addrErr) || addrErr.Code != tickets.CodeServerStamped {
			t.Fatalf("%s: err = %v, want code %s", flag, err, tickets.CodeServerStamped)
		}
		if raw, _ := os.ReadFile(ticketMD); string(raw) != epicTicketMD {
			t.Errorf("%s: ticket.md changed:\n%s", flag, raw)
		}
	}
}

func TestTicketsSet_MapEpicTakesCompletedAtAndKeepsTheFirstValue(t *testing.T) {
	d, ticketMD := setupEpicSetProject(t)
	if err := execute([]string{"tickets", "set", "mine:e", "--kind=map"}, d); err != nil {
		t.Fatalf("set kind: %v", err)
	}
	if err := execute([]string{"tickets", "set", "mine:e", "--completed-at=2026-02-03T04:05:06Z"}, d); err != nil {
		t.Fatalf("set completed-at: %v", err)
	}
	if err := execute([]string{"tickets", "set", "mine:e", "--completed-at=now"}, d); err != nil {
		t.Fatalf("set completed-at now: %v", err)
	}
	raw, _ := os.ReadFile(ticketMD)
	if !strings.Contains(string(raw), "completed_at: 2026-02-03T04:05:06Z") || strings.Count(string(raw), "completed_at:") != 1 {
		t.Errorf("want the first completed_at kept:\n%s", raw)
	}
	if err := execute([]string{"tickets", "set", "mine:e", "--completed-at=yesterday"}, d); err == nil {
		t.Error("bad timestamp: want a refusal")
	}
}

func TestTicketsSet_EpicAddressValidatesAgainstTheEpicSchema(t *testing.T) {
	d, ticketMD := setupEpicSetProject(t)
	// done is a valid ticket status but not an epic one.
	if err := execute([]string{"tickets", "set", "mine:e", "--status=done"}, d); err == nil {
		t.Fatal("status done: want a refusal")
	}
	if err := execute([]string{"tickets", "set", "mine:e", "--kind=bogus"}, d); err == nil {
		t.Fatal("kind bogus: want a refusal")
	}
	if raw, _ := os.ReadFile(ticketMD); string(raw) != epicTicketMD {
		t.Errorf("ticket.md changed:\n%s", raw)
	}
}

func TestTicketsSet_UnknownEpicRefuses(t *testing.T) {
	d, _ := setupEpicSetProject(t)
	err := execute([]string{"tickets", "set", "mine:nope", "--status=open"}, d)
	var addrErr *tickets.AddressError
	if !errors.As(err, &addrErr) || addrErr.Code != tickets.CodeUnknownEpic {
		t.Fatalf("err = %v, want code %s", err, tickets.CodeUnknownEpic)
	}
}

func TestTicketsSection_EpicAddressWritesBodySectionAndKeepsFrontmatter(t *testing.T) {
	d, ticketMD := setupEpicSetProject(t)

	if err := execute([]string{"tickets", "section", "mine:e", "Review findings", "all good"}, d); err != nil {
		t.Fatalf("section: %v", err)
	}
	raw, _ := os.ReadFile(ticketMD)
	for _, want := range []string{"## Review findings", "all good", "kind: loop", "custom: keep-me", "body words"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("ticket.md missing %q:\n%s", want, raw)
		}
	}
	if got := d.stdout.(*bytes.Buffer).String(); got != "mine:e\n" {
		t.Errorf("printed %q, want %q", got, "mine:e\n")
	}
}
