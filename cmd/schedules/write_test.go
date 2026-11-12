package schedules

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// session runs `gx schedules <args>` against one server, with stdin as given.
type session struct {
	t *testing.T
	h *servertest.Harness
}

func startSession(t *testing.T) *session {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	return &session{t: t, h: servertest.StartWithStore(t, store)}
}

func (s *session) run(stdin string, args ...string) (string, error) {
	s.t.Helper()
	var out bytes.Buffer
	env := cmdbase.Env{Stdout: &out, Stdin: strings.NewReader(stdin), NewClient: func() (*apiclient.Client, error) { return s.h.Client, nil }}
	err := cmdtest.Run(s.t, env, args, New(Deps{Env: env}))
	return out.String(), err
}

func TestCreate_EveryFlagRoundTripsAndPrintsTheAddress(t *testing.T) {
	s := startSession(t)
	out, err := s.run("", "schedules", "create", "--name", "Nightly", "--prompt", "check the logs", "--day", "mon,tue", "--day", "friday",
		"--time", "09:00", "--time", "17:30", "--project", "proj", "--harness", "codex", "--late-limit", "3")
	if err != nil || strings.TrimSpace(out) != "proj:nightly" {
		t.Fatalf("out = %q err = %v", out, err)
	}
	res, err := s.h.Client.Schedules.Show(context.Background(), "proj", "nightly")
	if err != nil || res.Refused {
		t.Fatalf("show = %+v err = %v", res, err)
	}
	got := res.Schedule
	if !slices.Equal(got.Days, []string{"mon", "tue", "fri"}) || !slices.Equal(got.Times, []string{"09:00", "17:30"}) || got.Harness != "codex" || got.LateLimitHours != 3 || res.Prompt != "check the logs" {
		t.Errorf("schedule = %+v prompt = %q", got, res.Prompt)
	}
}

func TestCreate_DayWordsExpandAndPromptReadsStdin(t *testing.T) {
	s := startSession(t)
	cases := map[string][]string{
		"weekdays":  {"mon", "tue", "wed", "thu", "fri"},
		"weekends":  {"sat", "sun"},
		"every-day": {"sun", "mon", "tue", "wed", "thu", "fri", "sat"},
	}
	for word, want := range cases {
		name := "Run " + word
		if _, err := s.run("from stdin\n", "schedules", "create", "--name", name, "--prompt", "-", "--day", word, "--time", "09:00", "--project", "proj"); err != nil {
			t.Fatal(err)
		}
		res, err := s.h.Client.Schedules.Show(context.Background(), "proj", strings.ReplaceAll(strings.ToLower(name), " ", "-"))
		if err != nil || res.Refused {
			t.Fatalf("%s: show = %+v err = %v", word, res, err)
		}
		slices.Sort(want)
		got := slices.Clone(res.Schedule.Days)
		slices.Sort(got)
		if !slices.Equal(got, want) || res.Prompt != "from stdin" {
			t.Errorf("%s: days = %v prompt = %q", word, res.Schedule.Days, res.Prompt)
		}
	}
}

func TestCreate_RefusalExitsNonZeroNamingTheReason(t *testing.T) {
	s := startSession(t)
	_, err := s.run("", "schedules", "create", "--name", "Nightly", "--prompt", "x", "--day", "mon", "--project", "proj")
	if err == nil || !strings.Contains(err.Error(), "no-times") {
		t.Errorf("err = %v, want no-times", err)
	}
	out, err := s.run("", "schedules", "create", "--name", "Nightly", "--prompt", "x", "--day", "mon", "--project", "proj", "--json")
	cmdtest.RequireRefusalExit(t, err)
	if !strings.Contains(out, `"no-times"`) {
		t.Errorf("json = %q", out)
	}
}

func TestCreate_JSONPrintsTheSchedule(t *testing.T) {
	s := startSession(t)
	out, err := s.run("", "schedules", "create", "--name", "Nightly", "--prompt", "x", "--day", "mon", "--time", "09:00", "--project", "proj", "--json")
	if err != nil || !strings.Contains(out, `"slug": "nightly"`) && !strings.Contains(out, `"slug":"nightly"`) || !strings.Contains(out, `"via"`) {
		t.Errorf("json = %q err = %v", out, err)
	}
}

func TestEdit_ChangesOnlyTheFlagsGiven(t *testing.T) {
	s := startSession(t)
	if _, err := s.run("", "schedules", "create", "--name", "Nightly", "--prompt", "check", "--day", "mon", "--time", "09:00", "--project", "proj"); err != nil {
		t.Fatal(err)
	}
	out, err := s.run("", "schedules", "edit", "proj:nightly", "--name", "Evening", "--time", "18:00", "--late-limit", "4")
	if err != nil || strings.TrimSpace(out) != "proj:nightly" {
		t.Fatalf("out = %q err = %v", out, err)
	}
	res, err := s.h.Client.Schedules.Show(context.Background(), "proj", "nightly")
	if err != nil || res.Refused {
		t.Fatalf("show = %+v err = %v", res, err)
	}
	got := res.Schedule
	if got.Name != "Evening" || !slices.Equal(got.Times, []string{"18:00"}) || !slices.Equal(got.Days, []string{"mon"}) || got.LateLimitHours != 4 || res.Prompt != "check" {
		t.Errorf("schedule = %+v prompt = %q", got, res.Prompt)
	}
}

func TestSwitch_OnAndOffWithJSONChangeOnlyTheSwitch(t *testing.T) {
	s := startSession(t)
	if _, err := s.run("", "schedules", "create", "--name", "Nightly", "--prompt", "check", "--day", "mon", "--time", "09:00", "--project", "proj"); err != nil {
		t.Fatal(err)
	}
	out, err := s.run("", "schedules", "switch", "proj:nightly", "off")
	if err != nil || strings.TrimSpace(out) != "proj:nightly" {
		t.Fatalf("out = %q err = %v", out, err)
	}
	res, err := s.h.Client.Schedules.Show(context.Background(), "proj", "nightly")
	if err != nil || res.Schedule.Switch || res.Schedule.Name != "Nightly" {
		t.Fatalf("show after off = %+v err = %v", res, err)
	}
	out, err = s.run("", "schedules", "switch", "proj:nightly", "on", "--json")
	if err != nil || !strings.Contains(out, `"via"`) {
		t.Fatalf("json = %q err = %v", out, err)
	}
	res, err = s.h.Client.Schedules.Show(context.Background(), "proj", "nightly")
	if err != nil || !res.Schedule.Switch {
		t.Errorf("show after on = %+v err = %v", res, err)
	}
}

func TestSwitch_RefusalsExitNonZero(t *testing.T) {
	s := startSession(t)
	out, err := s.run("", "schedules", "switch", "proj:nope", "on", "--json")
	cmdtest.RequireRefusalExit(t, err)
	if !strings.Contains(out, `"unknown-schedule"`) {
		t.Errorf("json = %q", out)
	}
	if _, err := s.run("", "schedules", "switch", "proj:nope", "maybe"); err == nil {
		t.Error("a bad on|off argument was accepted")
	}
}

func TestDelete_RemovesTheScheduleAndRefusesAnUnknownOne(t *testing.T) {
	s := startSession(t)
	if _, err := s.run("", "schedules", "create", "--name", "Nightly", "--prompt", "check", "--day", "mon", "--time", "09:00", "--project", "proj"); err != nil {
		t.Fatal(err)
	}
	out, err := s.run("", "schedules", "delete", "proj:nightly", "--json")
	if err != nil || !strings.Contains(out, `"refused": false`) && !strings.Contains(out, `"refused":false`) {
		t.Fatalf("json = %q err = %v", out, err)
	}
	res, err := s.h.Client.Schedules.Show(context.Background(), "proj", "nightly")
	if err != nil || !res.Refused {
		t.Errorf("show after delete = %+v err = %v", res, err)
	}
	out, err = s.run("", "schedules", "delete", "proj:nightly", "--json")
	cmdtest.RequireRefusalExit(t, err)
	if !strings.Contains(out, `"unknown-schedule"`) {
		t.Errorf("json = %q", out)
	}
}

func TestEdit_ProjectChangeIsRefused(t *testing.T) {
	s := startSession(t)
	if _, err := s.run("", "schedules", "create", "--name", "Nightly", "--prompt", "check", "--day", "mon", "--time", "09:00", "--project", "proj"); err != nil {
		t.Fatal(err)
	}
	_, err := s.run("", "schedules", "edit", "proj:nightly", "--project", "scratch")
	if err == nil || !strings.Contains(err.Error(), "project-read-only") {
		t.Errorf("err = %v, want project-read-only", err)
	}
}
