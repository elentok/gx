package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Every command that names a ticket in human output prints its canonical
// address, never the bare id or the file path.

func assertNamesByAddress(t *testing.T, out, epicPath string) {
	t.Helper()
	want := epicTicketLabel(epicPath, "01")
	if !strings.HasPrefix(out, want+": ") {
		t.Errorf("output = %q, want it to start with canonical address %q", out, want)
	}
}

func TestRunTicketsLand_TextNamesTicketByAddress(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	f.in.JSON = false
	out, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	assertNamesByAddress(t, out, f.in.EpicPath)
}

func TestRunTicketsLand_AbortTextNamesTicketByAddress(t *testing.T) {
	t.Parallel()
	f := newLandFixture(t, ticketWith("claimed", ""))
	f.in.JSON = false
	pendingConflict(t, f)
	f.deps.AbortCherryPick = func(string) error { return nil }
	f.in.Abort = true
	out, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	assertNamesByAddress(t, out, f.in.EpicPath)
}

func TestRunTicketsReset_TextNamesTicketByAddress(t *testing.T) {
	t.Parallel()
	f := newResetFixture(t, ticketWith("claimed", ""))
	f.in.JSON = false
	out, err := f.run()
	if err != nil {
		t.Fatal(err)
	}
	assertNamesByAddress(t, out, f.in.EpicPath)
}

func TestRunTicketsUnpark_TextNamesTicketByAddress(t *testing.T) {
	t.Parallel()
	epicPath, _ := unparkFixture(t, parkedTicket)
	var stdout, stderr bytes.Buffer
	if err := runTicketsUnpark(epicPath, "01", false, time.Now(), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	assertNamesByAddress(t, stdout.String(), epicPath)
}

func TestRunTicketsAdd_PrintsAddressNotPath(t *testing.T) {
	t.Parallel()
	epicPath, _ := unparkFixture(t, parkedTicket)
	var stdout bytes.Buffer
	if err := runTicketsAdd(epicPath, "", "do-thing", &stdout); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(stdout.String())
	if want := epicTicketLabel(epicPath, "02"); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if strings.Contains(got, string(filepath.Separator)+"issues") {
		t.Errorf("output %q looks like a path", got)
	}
}
