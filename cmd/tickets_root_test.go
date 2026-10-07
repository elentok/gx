package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

// isolateTicketStore points HOME and XDG_DATA_HOME at temp dirs (so neither the
// user's config.json nor their real store is read) and returns the store path.
// Not parallel-safe: it sets env vars.
func isolateTicketStore(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	return filepath.Join(os.Getenv("XDG_DATA_HOME"), "gx", "tickets")
}

// addProject writes <store>/<name>/project.json pointing at repoPath and
// returns the project directory.
func addProject(t *testing.T, store, name, repoPath string) string {
	t.Helper()
	dir := filepath.Join(store, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]string{"name": name, "repo": repoPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runIn(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return cwd, nil },
	}
	err := execute(args, d)
	return stdout.String(), err
}

func TestExecute_TicketsRoot_ResolvesIntoStoreByProjectFile(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	addProject(t, store, "other", filepath.Join(t.TempDir(), "elsewhere"))
	project := addProject(t, store, "mine", repo)

	out, err := runIn(t, repo, "tickets", "root")
	if err != nil {
		t.Fatalf("execute tickets root: %v", err)
	}
	if want := project + "\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestExecute_TicketsRoot_HonorsConfiguredStorePath(t *testing.T) {
	isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	custom := filepath.Join(t.TempDir(), "custom-store")
	project := addProject(t, custom, "mine", repo)
	cfgDir := filepath.Join(os.Getenv("HOME"), ".config", "gx")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, cfgDir, "config.json", `{"ticket-store":{"path":"`+custom+`"}}`)

	out, err := runIn(t, repo, "tickets", "root")
	if err != nil {
		t.Fatalf("execute tickets root: %v", err)
	}
	if want := project + "\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestExecute_TicketsRoot_NoProjectRefusesWithProjectAddHint(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	addProject(t, store, "other", filepath.Join(t.TempDir(), "elsewhere"))

	for _, args := range [][]string{{"tickets", "root"}, {"tickets", "epics"}} {
		out, err := runIn(t, repo, args...)
		if err == nil {
			t.Fatalf("%v: expected error, got nil", args)
		}
		if !strings.Contains(err.Error(), "gx project add .") {
			t.Errorf("%v: error = %q, want project add hint", args, err.Error())
		}
		if out != "" {
			t.Errorf("%v: stdout = %q, want empty", args, out)
		}
	}
}

func TestExecute_TicketsRoot_NotAGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var stdout, stderr bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: &stderr,
		getwd:  func() (string, error) { return dir, nil },
	}

	err := execute([]string{"tickets", "root"}, d)
	if err == nil {
		t.Fatal("expected error when cwd is outside a git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a git repo") {
		t.Errorf("error = %q, want it to mention not being inside a git repo", err.Error())
	}
}
