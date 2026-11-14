package files

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

type fixture struct {
	h    *servertest.Harness
	repo string
}

func start(t *testing.T) fixture {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	if err := os.MkdirAll(filepath.Join(repo, "docs/plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"docs/plans/host-files.md", ".env"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return fixture{h: servertest.StartWithStore(t, store, servertest.WithTestHolder()), repo: repo}
}

// run runs `gx files <args>` with cwd as the caller's working folder.
func (f fixture) run(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	env := cmdbase.Env{
		Stdout:    &out,
		Getwd:     func() (string, error) { return cwd, nil },
		NewClient: func() (*apiclient.Client, error) { return f.h.Client, nil },
	}
	err := cmdtest.Run(t, env, append([]string{"files"}, args...), New(Deps{Env: env}))
	return out.String(), err
}

type jsonOut struct {
	Files []struct {
		Candidate string `json:"candidate"`
		Verdict   string `json:"verdict"`
		Kind      string `json:"kind"`
	} `json:"files"`
	Reason string `json:"reason"`
}

func decode(t *testing.T, s string) jsonOut {
	t.Helper()
	var j jsonOut
	if err := json.Unmarshal([]byte(s), &j); err != nil {
		t.Fatalf("json %q: %v", s, err)
	}
	return j
}

func TestResolve_JSONUsesTheWorkingFolderAsBase(t *testing.T) {
	f := start(t)
	out, err := f.run(t, f.repo, "resolve", "--json", "docs/plans/host-files.md", "notes/none.md", ".env")
	if err != nil {
		t.Fatal(err)
	}
	j := decode(t, out)
	want := []string{"found", "missing", "missing"}
	if len(j.Files) != 3 {
		t.Fatalf("json = %q", out)
	}
	for i, w := range want {
		if j.Files[i].Verdict != w {
			t.Errorf("entry %d (%s) = %s, want %s", i, j.Files[i].Candidate, j.Files[i].Verdict, w)
		}
	}
	if j.Files[0].Kind != "markdown" || !strings.Contains(out, `"via"`) {
		t.Errorf("json = %q, want kind markdown and provenance", out)
	}
}

func TestResolve_TextPrintsOneLinePerPath(t *testing.T) {
	f := start(t)
	out, err := f.run(t, f.repo, "resolve", "docs/plans/host-files.md", "notes/none.md")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "found\t") || !strings.Contains(lines[0], "markdown") || lines[1] != "missing\tnotes/none.md" {
		t.Errorf("text = %q", out)
	}
}

func TestResolve_BaseFlagsOverrideTheWorkingFolder(t *testing.T) {
	f := start(t)
	elsewhere := t.TempDir()
	for name, flags := range map[string][]string{
		"base":    {"--base", f.repo},
		"project": {"--project", "proj"},
	} {
		out, err := f.run(t, elsewhere, append([]string{"resolve", "--json"}, append(flags, "docs/plans/host-files.md")...)...)
		if err != nil || decode(t, out).Files[0].Verdict != "found" {
			t.Errorf("%s: out = %q err = %v", name, out, err)
		}
	}
	out, _ := f.run(t, elsewhere, "resolve", "--json", "docs/plans/host-files.md")
	if decode(t, out).Files[0].Verdict != "missing" {
		t.Errorf("cwd elsewhere = %q, want missing", out)
	}
}

func TestResolve_SessionFlagUsesTheSessionFolder(t *testing.T) {
	f := start(t)
	created, err := f.h.Client.Sessions.Create(context.Background(), wire.CreateSessionRequest{Project: "proj", Dir: f.repo, Harness: string(agentfake.HarnessKind)})
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	out, err := f.run(t, t.TempDir(), "resolve", "--json", "--session", created.Session.ID, "docs/plans/host-files.md")
	if err != nil || decode(t, out).Files[0].Verdict != "found" {
		t.Errorf("out = %q err = %v", out, err)
	}
}

func TestResolve_TooManyPathsExitNonZero(t *testing.T) {
	f := start(t)
	args := []string{"resolve"}
	for i := range 201 {
		args = append(args, "/nope/"+strconv.Itoa(i))
	}
	if _, err := f.run(t, f.repo, args...); err == nil || !strings.Contains(err.Error(), "too-many-candidates") {
		t.Errorf("err = %v, want too-many-candidates", err)
	}
	out, err := f.run(t, f.repo, append(args, "--json")...)
	cmdtest.RequireRefusalExit(t, err)
	if decode(t, out).Reason != "too-many-candidates" {
		t.Errorf("json = %q", out)
	}
}
