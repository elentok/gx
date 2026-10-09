package claudedoctor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pass(Input) (string, bool) { return "ok", true }
func fail(Input) (string, bool) { return "broken", false }

func TestRun_DefaultTableAgainstFixtures(t *testing.T) {
	t.Parallel()
	src, err := FixtureSource()
	if err != nil {
		t.Fatalf("FixtureSource: %v", err)
	}
	results := Run(Table, "headless", src)
	if len(results) == 0 {
		t.Fatal("no headless checks ran")
	}
	for _, r := range results {
		if r.Status == Fail {
			t.Errorf("%s failed on fixtures: %s", r.Name, r.Detail)
		}
	}
}

func TestRun_SkipsDependentsOfFailedOrSkippedChecks(t *testing.T) {
	t.Parallel()
	table := []Check{
		{Runner: "headless", Name: "a", Mode: Fixture, Run: fail},
		{Runner: "headless", Name: "b", Mode: Fixture, DependsOn: []string{"a"}, Run: pass},
		{Runner: "headless", Name: "c", Mode: Fixture, DependsOn: []string{"b"}, Run: pass},
		{Runner: "headless", Name: "d", Mode: Fixture, Run: pass},
	}
	got := Run(table, "headless", Source{})
	want := []Status{Fail, Skip, Skip, Pass}
	for i, r := range got {
		if r.Status != want[i] {
			t.Errorf("%s: status %s, want %s", r.Name, r.Status, want[i])
		}
	}
	if got[1].Detail != "needs a" {
		t.Errorf("skip detail = %q, want %q", got[1].Detail, "needs a")
	}
}

func TestRun_LiveChecksSkipOnFixtures(t *testing.T) {
	t.Parallel()
	table := []Check{{Runner: "headless", Name: "live-one", Mode: Live, Run: pass}}

	if got := Run(table, "headless", Source{}); got[0].Status != Skip {
		t.Errorf("fixture source: status %s, want SKIP", got[0].Status)
	}
	if got := Run(table, "headless", Source{Live: true}); got[0].Status != Pass {
		t.Errorf("live source: status %s, want PASS", got[0].Status)
	}
}

func TestRun_OnlyRunsTheRequestedRunner(t *testing.T) {
	t.Parallel()
	table := []Check{
		{Runner: "headless", Name: "h", Mode: Fixture, Run: pass},
		{Runner: "pty", Name: "p", Mode: Fixture, Run: pass},
	}
	got := Run(table, "pty", Source{})
	if len(got) != 1 || got[0].Name != "p" || got[0].Runner != "pty" {
		t.Fatalf("results = %+v, want only pty/p", got)
	}
}

func TestRun_PassesSessionEventsAndFailsOnSessionError(t *testing.T) {
	t.Parallel()
	var seen int
	table := []Check{
		{Runner: "headless", Name: "reads", Mode: Fixture, Session: "init", Run: func(in Input) (string, bool) {
			seen = len(in.Events)
			return "ok", true
		}},
		{Runner: "headless", Name: "missing", Mode: Fixture, Session: "nope", Run: pass},
	}
	src := Source{Session: func(name string) ([]json.RawMessage, error) {
		if name == "init" {
			return []json.RawMessage{json.RawMessage(`{}`), json.RawMessage(`{}`)}, nil
		}
		return nil, errors.New("no such session")
	}}
	got := Run(table, "headless", src)
	if got[0].Status != Pass || seen != 2 {
		t.Errorf("reads: status %s, saw %d events", got[0].Status, seen)
	}
	if got[1].Status != Fail || !strings.Contains(got[1].Detail, "no such session") {
		t.Errorf("missing: %+v, want FAIL naming the error", got[1])
	}
}

func TestClaudeVersionCheck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		version string
		ok      bool
	}{
		{"2.1.292", true},
		{"2.1.300", true},
		{"2.2.0", true},
		{"2.1.291", false},
		{"1.9.999", false},
		{"", false},
	}
	for _, c := range cases {
		if _, ok := checkClaudeVersion(Input{ClaudeVersion: c.version}); ok != c.ok {
			t.Errorf("version %q: ok=%v, want %v", c.version, ok, c.ok)
		}
	}
}

func TestNewerThanFixtures(t *testing.T) {
	t.Parallel()
	if !NewerThanFixtures("2.1.300", "2.1.293") {
		t.Error("2.1.300 should be newer than 2.1.293 fixtures")
	}
	if NewerThanFixtures("2.1.293", "2.1.293") || NewerThanFixtures("2.1.200", "2.1.293") {
		t.Error("same or older claude must not warn")
	}
	if NewerThanFixtures("", "2.1.293") {
		t.Error("unknown installed version must not warn")
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()
	if got := ParseVersion("2.1.293 (Claude Code)\n"); got != "2.1.293" {
		t.Errorf("ParseVersion = %q", got)
	}
}

func TestFixtureVersionComesFromHeaders(t *testing.T) {
	t.Parallel()
	src, err := FixtureSource()
	if err != nil {
		t.Fatalf("FixtureSource: %v", err)
	}
	if src.ClaudeVersion != "2.1.293" {
		t.Errorf("fixture version = %q, want 2.1.293", src.ClaudeVersion)
	}
	events, err := src.Session("init")
	if err != nil {
		t.Fatalf("Session(init): %v", err)
	}
	if len(events) != 1 {
		t.Errorf("init events = %d, want 1 (header excluded)", len(events))
	}
}

func TestRecord_WritesSessionsWithVersionHeaderAndRoundTrips(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	live := Source{
		Live:          true,
		ClaudeVersion: "2.1.300",
		Session: func(string) ([]json.RawMessage, error) {
			return []json.RawMessage{json.RawMessage(`{"type":"system","subtype":"init"}`)}, nil
		},
	}
	rec := Record(live, dir)
	if _, err := rec.Session("init"); err != nil {
		t.Fatalf("recorded Session: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "init.jsonl"))
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	want := "{\"claude_version\":\"2.1.300\"}\n{\"type\":\"system\",\"subtype\":\"init\"}\n"
	if string(data) != want {
		t.Fatalf("recording = %q, want %q", data, want)
	}

	loaded, err := LoadFixtures(os.DirFS(dir))
	if err != nil {
		t.Fatalf("LoadFixtures: %v", err)
	}
	if loaded.ClaudeVersion != "2.1.300" {
		t.Errorf("round-trip version = %q", loaded.ClaudeVersion)
	}
}

func TestLoadFixtures_RejectsMissingHeader(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.jsonl"), []byte(`{"type":"system"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFixtures(os.DirFS(dir)); err == nil {
		t.Fatal("expected an error for a fixture without a claude_version header")
	}
}
