package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/elentok/gx/claudedoctor"
	"github.com/elentok/gx/nativerunner"
)

func fixtureDoctorSource(t *testing.T) claudedoctor.Source {
	t.Helper()
	src, err := claudedoctor.FixtureSource()
	if err != nil {
		t.Fatalf("FixtureSource: %v", err)
	}
	return src
}

func TestRunClaudeDoctor_PrintsHeaderAndResultLines(t *testing.T) {
	t.Parallel()
	out, errOut := &strings.Builder{}, &strings.Builder{}
	d := deps{stdout: out, stderr: errOut}

	err := runClaudeDoctor(d, claudeDoctorOptions{runner: "headless"}, fixtureDoctorSource(t), "")
	if err != nil {
		t.Fatalf("runClaudeDoctor: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	wantHeader := "claude 2.1.293 (MinClaudeVersion " + nativerunner.MinClaudeVersion + ")"
	if lines[0] != wantHeader {
		t.Errorf("header = %q, want %q", lines[0], wantHeader)
	}
	if lines[1] != "PASS claude-version - 2.1.293 >= "+nativerunner.MinClaudeVersion {
		t.Errorf("result line = %q", lines[1])
	}
	if errOut.Len() != 0 {
		t.Errorf("unexpected stderr: %q", errOut.String())
	}
}

func TestRunClaudeDoctor_FailExitsOne(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{stdout: out, stderr: &strings.Builder{}}
	src := claudedoctor.Source{Live: true, ClaudeVersion: "1.0.0"}

	err := runClaudeDoctor(d, claudeDoctorOptions{runner: "headless"}, src, "")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("err = %v, want ExitError{1}", err)
	}
	if !strings.Contains(out.String(), "FAIL claude-version - ") {
		t.Errorf("output missing FAIL line: %q", out.String())
	}
}

func TestRunClaudeDoctor_JSON(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{stdout: out, stderr: &strings.Builder{}}

	opts := claudeDoctorOptions{runner: "headless", json: true, table: claudedoctor.Table[:1]}
	if err := runClaudeDoctor(d, opts, fixtureDoctorSource(t), ""); err != nil {
		t.Fatalf("runClaudeDoctor: %v", err)
	}
	var got []map[string]string
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out.String())
	}
	want := map[string]string{"runner": "headless", "name": "claude-version", "mode": "fixture", "status": "PASS", "detail": "2.1.293 >= " + nativerunner.MinClaudeVersion}
	if len(got) != 1 || len(got[0]) != len(want) {
		t.Fatalf("got %v, want [%v]", got, want)
	}
	for k, v := range want {
		if got[0][k] != v {
			t.Errorf("%s = %q, want %q", k, got[0][k], v)
		}
	}
}

func TestRunClaudeDoctor_JSONIsEmptyArrayForRunnerWithoutChecks(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{stdout: out, stderr: &strings.Builder{}}

	if err := runClaudeDoctor(d, claudeDoctorOptions{runner: "pty", json: true}, fixtureDoctorSource(t), ""); err != nil {
		t.Fatalf("runClaudeDoctor: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "[]" {
		t.Errorf("output = %q, want []", got)
	}
}

func TestRunClaudeDoctor_WarnsWhenLiveClaudeIsNewerThanFixtures(t *testing.T) {
	t.Parallel()
	errOut := &strings.Builder{}
	d := deps{stdout: &strings.Builder{}, stderr: errOut}
	src := claudedoctor.Source{Live: true, ClaudeVersion: "2.1.400"}

	opts := claudeDoctorOptions{runner: "headless", table: claudedoctor.Table[:1]}
	if err := runClaudeDoctor(d, opts, src, "2.1.293"); err != nil {
		t.Fatalf("runClaudeDoctor: %v", err)
	}
	if !strings.Contains(errOut.String(), "newer than the test fixtures (2.1.293)") {
		t.Errorf("stderr = %q, want a newer-than-fixtures warning", errOut.String())
	}
}

func TestRunClaudeDoctor_RejectsUnknownRunner(t *testing.T) {
	t.Parallel()
	d := deps{stdout: &strings.Builder{}, stderr: &strings.Builder{}}
	for _, runner := range []string{"herdr", "auto", "bogus"} {
		if err := runClaudeDoctor(d, claudeDoctorOptions{runner: runner}, claudedoctor.Source{}, ""); err == nil {
			t.Errorf("--runner %s: expected an error", runner)
		}
	}
}
