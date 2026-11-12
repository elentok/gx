package schedules

import (
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/server/servertest"
)

func (s *session) createNightly() {
	s.t.Helper()
	if _, err := s.run("", "schedules", "create", "--name", "Nightly", "--prompt", "check", "--day", "mon", "--time", "09:00", "--project", "proj"); err != nil {
		s.t.Fatal(err)
	}
}

func TestRunNow_PrintsTheNewOneOffAddress(t *testing.T) {
	s := startSession(t)
	s.createNightly()
	out, err := s.run("", "schedules", "run-now", "proj:nightly")
	if err != nil || !strings.HasPrefix(strings.TrimSpace(out), "proj:one-offs/") {
		t.Fatalf("out = %q err = %v", out, err)
	}
}

func TestRunNow_RefusalsExitNonZeroWithTheReasonAndAddress(t *testing.T) {
	s := startSession(t)
	out, err := s.run("", "schedules", "run-now", "proj:nope", "--json")
	cmdtest.RequireRefusalExit(t, err)
	if !strings.Contains(out, `"unknown-schedule"`) {
		t.Errorf("json = %q", out)
	}

	s.createNightly()
	servertest.WriteTicketWith(t, s.h.TicketStore, "proj", "one-offs", "90", "previous", servertest.TicketOpts{
		Status: "paused", PauseReason: "question", Type: "prompt",
		Unique: "schedule:proj/nightly@" + s.h.Server.Schedules().MachineID(),
	})
	out, err = s.run("", "schedules", "run-now", "proj:nightly")
	if err == nil || !strings.Contains(err.Error(), "previous-run-needs-attention") || !strings.Contains(err.Error(), "proj:one-offs/90") {
		t.Errorf("err = %v out = %q", err, out)
	}
	if _, err := s.run("", "schedules", "run-now", "nightly"); err == nil {
		t.Error("an address without a project was accepted")
	}
}
