package server_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// checksFixture registers project "demo" on a fresh repo; extra is raw
// project.json members.
func checksFixture(t *testing.T, extra string) (h *servertest.Harness, repo string) {
	t.Helper()
	h = servertest.StartWithStore(t, t.TempDir())
	repo = testutil.TempRepo(t)
	if err := servertest.WriteEpic(h.TicketStore, "demo", ""); err != nil {
		t.Fatal(err)
	}
	if err := servertest.WriteProjectRepo(h.TicketStore, "demo", repo, extra); err != nil {
		t.Fatal(err)
	}
	return h, repo
}

// writeNested is testutil.WriteFile for a name whose directory may not exist.
func writeNested(t *testing.T, repo, name, content string) {
	t.Helper()
	testutil.Mkdir(t, filepath.Join(repo, filepath.Dir(name)))
	testutil.WriteFile(t, repo, name, content)
}

func demoChecks(t *testing.T, h *servertest.Harness) server.ProjectChecks {
	t.Helper()
	st, err := h.Client.ChecksStatus(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Projects) != 1 {
		t.Fatalf("projects = %+v", st.Projects)
	}
	return st.Projects[0]
}

func checkKind(t *testing.T, p server.ProjectChecks, kind string) server.KindCheck {
	t.Helper()
	for _, k := range p.Kinds {
		if k.Kind == kind {
			return k
		}
	}
	t.Fatalf("no kind %q in %+v", kind, p.Kinds)
	return server.KindCheck{}
}

func TestChecksStatus_RanksSourcesAndRunsNothing(t *testing.T) {
	h, repo := checksFixture(t, "")
	testutil.WriteFile(t, repo, "Makefile", "test:\n\ttouch ran.txt\n")
	testutil.WriteFile(t, repo, "justfile", "test:\n    touch ran.txt\n")
	testutil.WriteFile(t, repo, "package.json", `{"scripts":{"test":"touch ran.txt","build":"x"}}`)
	testutil.CommitAll(t, repo, "add tooling")

	p := demoChecks(t, h)
	test := checkKind(t, p, "test")
	if test.Recommended == nil || test.Recommended.Command != "make test" || test.Recommended.Source != "Makefile · test" {
		t.Errorf("recommended = %+v", test.Recommended)
	}
	want := []check.Candidate{{Command: "just test", Source: "justfile · test"}, {Command: "npm run test", Source: "package.json · test"}}
	if len(test.Candidates) != 2 || test.Candidates[0] != want[0] || test.Candidates[1] != want[1] {
		t.Errorf("candidates = %+v, want %+v", test.Candidates, want)
	}
	if _, err := os.Stat(filepath.Join(repo, "ran.txt")); err == nil {
		t.Error("detection ran a command")
	}
	if got := checkKind(t, p, "lint"); got.Recommended != nil || got.Default == "" {
		t.Errorf("lint = %+v, want no recommendation and a default line", got)
	}
}

func TestChecksStatus_BuildScriptIsNeverOffered(t *testing.T) {
	h, repo := checksFixture(t, "")
	testutil.WriteFile(t, repo, "package.json", `{"scripts":{"build":"x"}}`)
	testutil.CommitAll(t, repo, "add package")
	for _, k := range demoChecks(t, h).Kinds {
		if k.Recommended != nil || len(k.Candidates) != 0 {
			t.Errorf("%s = %+v, want none", k.Kind, k)
		}
	}
}

func TestChecksStatus_FindsWorkspaceScriptAndMiseTask(t *testing.T) {
	h, repo := checksFixture(t, "")
	testutil.WriteFile(t, repo, "package.json", `{"workspaces":["webui"]}`)
	testutil.WriteFile(t, repo, "pnpm-lock.yaml", "")
	writeNested(t, repo, "webui/package.json", `{"scripts":{"lint":"eslint ."}}`)
	testutil.WriteFile(t, repo, "mise.toml", "[tasks.typecheck]\nrun = \"tsc\"\n\n[tasks]\nfmt = \"gofmt\"\n")
	testutil.CommitAll(t, repo, "add tooling")

	p := demoChecks(t, h)
	if r := checkKind(t, p, "lint").Recommended; r == nil || r.Command != "pnpm --dir webui run lint" || r.Source != "webui/package.json · lint" {
		t.Errorf("lint = %+v", r)
	}
	if r := checkKind(t, p, "typecheck").Recommended; r == nil || r.Command != "mise run typecheck" || r.Source != "mise.toml · typecheck" {
		t.Errorf("typecheck = %+v", r)
	}
	if r := checkKind(t, p, "format").Recommended; r == nil || r.Command != "mise run fmt" {
		t.Errorf("format = %+v", r)
	}
}

func TestChecksStatus_MissingState(t *testing.T) {
	t.Run("no config is missing", func(t *testing.T) {
		h, _ := checksFixture(t, "")
		if p := demoChecks(t, h); !p.Missing || p.Trunk != "main" {
			t.Errorf("project = %+v", p)
		}
	})
	t.Run("empty repo file is config", func(t *testing.T) {
		h, repo := checksFixture(t, "")
		writeNested(t, repo, ".gx/checks.json", "{}")
		testutil.CommitAll(t, repo, "add checks")
		if p := demoChecks(t, h); p.Missing {
			t.Errorf("project = %+v", p)
		}
	})
	t.Run("empty checks key is config", func(t *testing.T) {
		h, _ := checksFixture(t, `,"checks":{}`)
		if p := demoChecks(t, h); p.Missing {
			t.Errorf("project = %+v", p)
		}
	})
	t.Run("invalid config is an error, not missing", func(t *testing.T) {
		h, repo := checksFixture(t, "")
		writeNested(t, repo, ".gx/checks.json", `{"lintt":"x"}`)
		testutil.CommitAll(t, repo, "add checks")
		if p := demoChecks(t, h); p.Missing || !strings.Contains(p.Error, "lintt") {
			t.Errorf("project = %+v", p)
		}
	})
	t.Run("scratch is never missing", func(t *testing.T) {
		h := servertest.StartWithStore(t, t.TempDir())
		st, err := h.Client.ChecksStatus(context.Background(), server.ScratchProject)
		if err != nil || len(st.Projects) != 1 || st.Projects[0].Missing {
			t.Errorf("scratch = %+v, %v", st, err)
		}
	})
}

func TestChecksStatus_CurrentState(t *testing.T) {
	h, _ := checksFixture(t, `,"checks":{"lint":false}`)
	if got := checkKind(t, demoChecks(t, h), "lint").State; got != "off" {
		t.Errorf("lint state = %q", got)
	}
	h, _ = checksFixture(t, `,"checks":{"test":{"full":"x"}}`)
	p := demoChecks(t, h)
	if got := checkKind(t, p, "test").State; got != "configured" {
		t.Errorf("test state = %q", got)
	}
	if got := checkKind(t, p, "lint").State; got != "gx-default" {
		t.Errorf("lint state = %q", got)
	}
}

func TestChecksStatus_VCSNoneSavesOnlyToProjectJSON(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	if err := servertest.WriteEpic(h.TicketStore, "notes", ""); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"notes","vcs":"none"}`
	if err := os.WriteFile(filepath.Join(h.TicketStore, "notes", "project.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := h.Client.ChecksStatus(context.Background(), "notes")
	if err != nil || len(st.Projects) != 1 {
		t.Fatalf("status = %+v, %v", st, err)
	}
	if got := st.Projects[0].SaveTargets; len(got) != 1 || got[0] != "project.json" {
		t.Errorf("save targets = %v", got)
	}
}

func TestChecksStatus_NoProjectListsEveryProject(t *testing.T) {
	h, _ := checksFixture(t, "")
	st, err := h.Client.ChecksStatus(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, p := range st.Projects {
		names[p.Project] = true
	}
	if !names["demo"] || !names[server.ScratchProject] {
		t.Errorf("projects = %v", names)
	}
}

func TestWeb_ChecksStatusIsServed(t *testing.T) {
	h, base := startWeb(t)
	if code, _ := webDo(t, h, http.MethodGet, base+"/v1/checks/status"); code != http.StatusOK {
		t.Errorf("GET /v1/checks/status = %d", code)
	}
}
