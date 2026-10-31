package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/codexdoctor"
)

func fixtureCodexSource(t *testing.T) codexdoctor.Source {
	t.Helper()
	src, err := codexdoctor.FixtureSource()
	if err != nil {
		t.Fatalf("FixtureSource: %v", err)
	}
	return src
}

func TestRunCodexDoctor_PrintsLinesAndPassesOnFixtures(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{stdout: out, stderr: &strings.Builder{}}
	if err := runCodexDoctor(d, codexDoctorOptions{}, fixtureCodexSource(t), ""); err != nil {
		t.Fatalf("runCodexDoctor: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if want := "codex 0.166.0 (MinCodexVersion 0.166.0)"; lines[0] != want {
		t.Errorf("header = %q, want %q", lines[0], want)
	}
	if want := "SKIP codex-version - live only"; lines[1] != want {
		t.Errorf("line 1 = %q, want %q", lines[1], want)
	}
	if !strings.HasPrefix(lines[2], "PASS rollout-format - ") {
		t.Errorf("line 2 = %q, want PASS rollout-format", lines[2])
	}
}

func TestRunCodexDoctor_ExitsOneOnFail(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{stdout: out, stderr: &strings.Builder{}}
	opts := codexDoctorOptions{table: []codexdoctor.Check{
		{Name: "broken", Mode: codexdoctor.Fixture, Run: func(codexdoctor.Input) (string, bool) { return "nope", false }},
	}}
	err := runCodexDoctor(d, opts, fixtureCodexSource(t), "")
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("err = %v, want ExitError{1}", err)
	}
	if !strings.Contains(out.String(), "FAIL broken - nope\n") {
		t.Errorf("output = %q, want a FAIL line", out.String())
	}
}

func TestRunCodexDoctor_JSON(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{stdout: out, stderr: &strings.Builder{}}
	if err := runCodexDoctor(d, codexDoctorOptions{json: true}, fixtureCodexSource(t), ""); err != nil {
		t.Fatalf("runCodexDoctor: %v", err)
	}
	var got []map[string]string
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out.String())
	}
	if len(got) != len(codexdoctor.Table) || got[1]["name"] != "rollout-format" || got[1]["mode"] != "fixture" || got[1]["status"] != "PASS" {
		t.Errorf("results = %v", got)
	}
}

func TestRunCodexDoctor_WarnsWhenLiveCodexIsNewerThanFixtures(t *testing.T) {
	t.Parallel()
	stderr := &strings.Builder{}
	d := deps{stdout: &strings.Builder{}, stderr: stderr}
	src := fixtureCodexSource(t)
	src.Live, src.CodexVersion = true, "0.170.0"
	if err := runCodexDoctor(d, codexDoctorOptions{table: []codexdoctor.Check{}}, src, "0.166.0"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "codex 0.170.0 is newer than the test fixtures (0.166.0)") {
		t.Errorf("stderr = %q, want newer-than-fixtures warning", stderr.String())
	}
}

func TestRunCodexDoctor_RecordedSessionsLoadBack(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	frames := []codexdoctor.Frame{{T: 0.5, Data: []byte("\x1b]7501;state=idle:app=codex\a")}}
	src := codexdoctor.Record(codexdoctor.Source{
		Session: func(string) (codexdoctor.Recording, error) { return codexdoctor.Recording{Frames: frames}, nil },
	}, dir)
	opts := codexDoctorOptions{table: []codexdoctor.Check{
		{Name: "turny", Mode: codexdoctor.Fixture, Session: codexdoctor.TurnSession, Run: func(in codexdoctor.Input) (string, bool) { return "", len(in.Frames) == 1 }},
	}}
	d := deps{stdout: &strings.Builder{}, stderr: &strings.Builder{}}
	if err := runCodexDoctor(d, opts, src, ""); err != nil {
		t.Fatal(err)
	}
	got, err := codexdoctor.ReadStream(dir + "/streams/turn.jsonl")
	if err != nil || len(got) != 1 || got[0].T != 0.5 || !bytes.Equal(got[0].Data, frames[0].Data) {
		t.Errorf("ReadStream = %v, %v, want the recorded frame", got, err)
	}
}

func helpOf(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	d := deps{env: envFor(t), stdout: &out, stderr: &out}
	if err := execute(append(args, "--help"), d); err != nil {
		t.Fatalf("%v --help: %v", args, err)
	}
	return out.String()
}

func TestCodexDoctor_HelpListsJSONLikeClaudeDoctor(t *testing.T) {
	t.Parallel()
	codex := helpOf(t, "codex", "doctor")
	claude := helpOf(t, "claude", "doctor")
	flagLine := func(help, flag string) string {
		for _, l := range strings.Split(help, "\n") {
			if strings.Contains(l, flag) {
				return strings.Join(strings.Fields(l), " ")
			}
		}
		return ""
	}
	for _, flag := range []string{"--json", "--record"} {
		if flagLine(codex, flag) == "" {
			t.Fatalf("codex doctor --help lists no %s:\n%s", flag, codex)
		}
		if got, want := strings.Fields(flagLine(codex, flag))[:2], strings.Fields(flagLine(claude, flag))[:2]; !slices.Equal(got, want) {
			t.Errorf("%s flag columns = %q, want %q like claude doctor", flag, got, want)
		}
	}
	if !strings.Contains(codex, "Usage:\n  gx codex doctor [flags]") && !strings.Contains(codex, "codex doctor [flags]") {
		t.Errorf("usage line missing:\n%s", codex)
	}
}
