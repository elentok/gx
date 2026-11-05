package ticketmove

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/tickettest"
)

func writeArchived(t *testing.T, dir, id, resolution string) {
	t.Helper()
	tickettest.WriteFile(t, filepath.Join(dir, tickets.ArchivedOneOffsEpic, "issues", id+"-one-off.md"),
		"---\nid: \""+id+"\"\nstatus: done\ntype: implement\n---\n\n## Resolution\n\n"+resolution)
}

func TestOriginalLine_RoundTrip(t *testing.T) {
	addr := tickets.Address{Project: "gx", Epic: tickets.OneOffsEpic, ID: "03"}
	body := "## Resolution\n\n- reason: done\n" + tickets.OriginalLine(addr) + "\n"
	if !tickets.NamesOriginal(body, addr) {
		t.Fatalf("NamesOriginal did not match its own line in %q", body)
	}
	if tickets.NamesOriginal(body, tickets.Address{Project: "gx", Epic: tickets.OneOffsEpic, ID: "04"}) {
		t.Fatal("NamesOriginal matched another address")
	}
}

func TestFindOriginal_ReadsFrontmatterField(t *testing.T) {
	dir := t.TempDir()
	tickettest.WriteFile(t, filepath.Join(dir, tickets.ArchivedOneOffsEpic, "issues", "01-one-off.md"),
		"---\nid: \"01\"\nstatus: done\ntype: implement\noriginal_address: p:one-offs/03\n---\n\n# 01\n")
	writeArchived(t, dir, "02", "- original: p:one-offs/04\n")

	for _, c := range []struct{ id, want string }{{"03", "01"}, {"04", "02"}} {
		got, _, err := tickets.FindOriginal("p", dir, tickets.Address{Project: "p", Epic: "one-offs", ID: c.id})
		if err != nil || got.ID != c.want {
			t.Fatalf("one-offs/%s: got %v, %v; want archived %s", c.id, got, err, c.want)
		}
	}
}

func TestPlanMove_KeepsOriginalAddress(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, tickets.OneOffsEpic, "issues", "03-job.md")
	tickettest.WriteFile(t, src, "---\nid: \"03\"\nstatus: done\ntype: implement\noriginal_address: p:one-offs/03\n---\n\n# 03\n")
	tickettest.WriteFile(t, filepath.Join(root, tickets.ArchivedOneOffsEpic, "issues", "01-other.md"),
		"---\nid: \"01\"\nstatus: done\ntype: implement\n---\n\n# 01\n")
	epics, err := tickets.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var tk tickets.Ticket
	for _, e := range epics {
		if e.Name == tickets.OneOffsEpic {
			tk = e.Tickets[0]
		}
	}
	addr := tickets.Address{Project: "p", Epic: tickets.OneOffsEpic, ID: "03"}
	plan, err := PlanMove(root, addr, tk, tickets.ArchivedOneOffsEpic, "", MoveOpts{Archive: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyMove(plan); err != nil {
		t.Fatal(err)
	}
	got, _, err := tickets.FindOriginal("p", root, addr)
	if err != nil || got.ID != "02" {
		t.Fatalf("got %v, %v; want archived 02 found with no Resolution line", got, err)
	}
}

func TestFindOriginal(t *testing.T) {
	dir := t.TempDir()
	writeArchived(t, dir, "01", "- original: p:one-offs/03\n")
	writeArchived(t, dir, "02", "- original: p:one-offs/04\n")
	writeArchived(t, dir, "03", "- original: p:one-offs/05\n")
	writeArchived(t, dir, "04", "- original: p:one-offs/05\n")

	t.Run("found", func(t *testing.T) {
		got, tk, err := tickets.FindOriginal("p", dir, tickets.Address{Project: "p", Epic: "one-offs", ID: "03"})
		if err != nil || got.String() != "p:one-offs-archived/01" || tk.Identifier != "01" {
			t.Fatalf("got %v, %q, %v", got, tk.Identifier, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		_, _, err := tickets.FindOriginal("p", dir, tickets.Address{Project: "p", Epic: "one-offs", ID: "09"})
		var ae *tickets.AddressError
		if !errors.As(err, &ae) || ae.Code != tickets.CodeUnknownTicket {
			t.Fatalf("err = %v, want %s", err, tickets.CodeUnknownTicket)
		}
	})
	t.Run("ambiguous", func(t *testing.T) {
		_, _, err := tickets.FindOriginal("p", dir, tickets.Address{Project: "p", Epic: "one-offs", ID: "05"})
		var ae *tickets.AddressError
		if !errors.As(err, &ae) || ae.Code != tickets.CodeAmbiguousOriginal {
			t.Fatalf("err = %v, want %s", err, tickets.CodeAmbiguousOriginal)
		}
	})
}
