package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

type onboardEnv struct {
	h     *servertest.Harness
	repo  string
	name  string
	out   bytes.Buffer
	errs  bytes.Buffer
	edits int
}

// newOnboardEnv adds a repo whose committed Makefile has a test target. Each
// scripted editor save is written into the temp file the editor opens.
func newOnboardEnv(t *testing.T) *onboardEnv {
	t.Helper()
	e := &onboardEnv{h: servertest.Start(t), repo: testutil.TempRepo(t)}
	testutil.WriteFile(t, e.repo, "Makefile", "test:\n\techo hi\n")
	testutil.CommitAll(t, e.repo, "add makefile")
	e.name = filepath.Base(e.repo)
	return e
}

func (e *onboardEnv) deps(t *testing.T, tty bool, answers string, saves ...string) deps {
	t.Helper()
	return deps{
		stdin:  strings.NewReader(answers),
		stdout: &e.out,
		stderr: &e.errs,
		getwd:  func() (string, error) { return e.repo, nil },
		getenv: func(string) string { return "vi" },
		loadConfig: func() (config.Config, error) {
			return config.Config{TicketStore: config.TicketStoreConfig{Path: e.h.TicketStore}}, nil
		},
		stdinIsTerminal: func() bool { return tty },
		runEditor: func(_, path string, _ io.Reader, _, _ io.Writer) error {
			if e.edits >= len(saves) {
				t.Fatalf("editor opened %d times, scripted %d", e.edits+1, len(saves))
			}
			e.edits++
			return os.WriteFile(path, []byte(saves[e.edits-1]), 0o600)
		},
	}
}

func (e *onboardEnv) add(t *testing.T, d deps, jsonOut bool) error {
	t.Helper()
	return runProjectAdd(context.Background(), e.h.Client, d, &e.out, jsonOut, server.AddProjectRequest{Path: e.repo})
}

func (e *onboardEnv) projectChecks(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.h.TicketStore, e.name, config.ProjectFileName))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func commitCount(t *testing.T, repo string) int {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-list", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Fields(string(out)))
}

func TestProjectAdd_AcceptAllCommitsChecksOnTrunk(t *testing.T) {
	e := newOnboardEnv(t)
	before := commitCount(t, e.repo)
	if err := e.add(t, e.deps(t, true, "1\n1\n"), false); err != nil {
		t.Fatal(err)
	}
	out := e.out.String()
	if !strings.Contains(out, "Makefile · test") || !strings.Contains(out, "make test") {
		t.Errorf("menu lacks the recommendation and source:\n%s", out)
	}
	for _, kind := range []string{"format", "lint", "typecheck", "test"} {
		if !strings.Contains(out, kind) {
			t.Errorf("menu lacks kind %s:\n%s", kind, out)
		}
	}
	if !strings.Contains(out, "Committed .gx/checks.json on ") {
		t.Errorf("output lacks the commit line:\n%s", out)
	}
	if got := commitCount(t, e.repo); got != before+1 {
		t.Errorf("commits = %d, want %d", got, before+1)
	}
	b, err := os.ReadFile(filepath.Join(e.repo, ".gx", "checks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]string
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["test"]["full"] != "make test" {
		t.Errorf("checks.json = %s, want only test: make test", b)
	}
}

func TestProjectAdd_ProjectJSONTargetMakesNoCommit(t *testing.T) {
	e := newOnboardEnv(t)
	before := commitCount(t, e.repo)
	if err := e.add(t, e.deps(t, true, "1\n2\n"), false); err != nil {
		t.Fatal(err)
	}
	if got := commitCount(t, e.repo); got != before {
		t.Errorf("commits = %d, want %d", got, before)
	}
	if body := e.projectChecks(t); !strings.Contains(body, `"checks"`) || !strings.Contains(body, "make test") {
		t.Errorf("project.json = %s, want checks with make test", body)
	}
}

func TestProjectOnboard_SkipSavesNothingAndAsksAgain(t *testing.T) {
	e := newOnboardEnv(t)
	if err := e.add(t, e.deps(t, true, "4\n"), false); err != nil {
		t.Fatal(err)
	}
	if body := e.projectChecks(t); strings.Contains(body, "checks") {
		t.Errorf("skip saved: %s", body)
	}
	e.out.Reset()
	if err := runProjectOnboard(context.Background(), e.h.Client, e.deps(t, true, "4\n"), e.name); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.out.String(), "Check config is missing") {
		t.Errorf("onboard did not ask again:\n%s", e.out.String())
	}
}

func TestProjectOnboard_EditInvalidThenDeclineSavesNothing(t *testing.T) {
	e := newOnboardEnv(t)
	d := e.deps(t, true, "2\nn\n", `{"nope": "x"}`)
	if err := e.add(t, d, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.errs.String(), "unknown kind") || !strings.Contains(e.out.String(), "Edit again?") {
		t.Errorf("stderr = %q, stdout = %q", e.errs.String(), e.out.String())
	}
	if body := e.projectChecks(t); strings.Contains(body, "checks") {
		t.Errorf("declined edit saved: %s", body)
	}
}

func TestProjectOnboard_EditValidSaves(t *testing.T) {
	e := newOnboardEnv(t)
	d := e.deps(t, true, "2\n2\n", `{"lint": {"full": "make lint"}}`)
	if err := e.add(t, d, false); err != nil {
		t.Fatal(err)
	}
	if body := e.projectChecks(t); !strings.Contains(body, "make lint") || strings.Contains(body, "make test") {
		t.Errorf("project.json = %s, want the edited config", body)
	}
}

func TestProjectOnboard_KeepDefaultsSavesEmptyConfig(t *testing.T) {
	e := newOnboardEnv(t)
	if err := e.add(t, e.deps(t, true, "3\n2\n"), false); err != nil {
		t.Fatal(err)
	}
	if body := e.projectChecks(t); !strings.Contains(body, `"checks": {}`) {
		t.Errorf("project.json = %s, want empty checks", body)
	}
	e.out.Reset()
	if err := runProjectOnboard(context.Background(), e.h.Client, e.deps(t, true, ""), e.name); err != nil {
		t.Fatal(err)
	}
	if got := e.out.String(); got != "nothing missing\n" {
		t.Errorf("onboard after save = %q", got)
	}
}

func TestProjectAdd_VCSNoneSavesToProjectJSONWithoutTargetPrompt(t *testing.T) {
	e := newOnboardEnv(t)
	req := server.AddProjectRequest{Path: t.TempDir(), Name: "loose", VCS: "none"}
	d := e.deps(t, true, "1\n")
	if err := runProjectAdd(context.Background(), e.h.Client, d, &e.out, false, req); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.out.String(), "Where to save?") {
		t.Errorf("vcs none was asked for a target:\n%s", e.out.String())
	}
	b, err := os.ReadFile(filepath.Join(e.h.TicketStore, "loose", config.ProjectFileName))
	if err != nil || !strings.Contains(string(b), `"checks"`) {
		t.Errorf("project.json = %s, %v", b, err)
	}
}

func TestProjectAdd_NoTerminalNeverPrompts(t *testing.T) {
	e := newOnboardEnv(t)
	if err := e.add(t, e.deps(t, false, "1\n1\n"), false); err != nil {
		t.Fatal(err)
	}
	out := e.out.String()
	if strings.Contains(out, "Choice") || !strings.Contains(out, "gx project onboard "+e.name) || !strings.Contains(out, "test") {
		t.Errorf("output = %q", out)
	}
}

func TestProjectAdd_JSONNeverPromptsAndStaysOneObject(t *testing.T) {
	e := newOnboardEnv(t)
	if err := e.add(t, e.deps(t, true, "1\n1\n"), true); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(e.out.Bytes(), &got); err != nil {
		t.Fatalf("output is not one JSON object: %v\n%s", err, e.out.String())
	}
	if got["onboard_command"] != "gx project onboard "+e.name || got["checks_missing"] == nil || got["name"] != e.name {
		t.Errorf("json = %v", got)
	}
}
