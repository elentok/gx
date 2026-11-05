package tickets

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/testutil"
)

func TestExecute_TicketsRoot_ResolvesIntoStoreByProjectFile(t *testing.T) {
	store := cmdtest.IsolateTicketStore(t)
	repo := testutil.TempRepo(t)
	cmdtest.AddProject(t, store, "other", filepath.Join(t.TempDir(), "elsewhere"))
	project := cmdtest.AddProject(t, store, "mine", repo)

	out, err := runIn(t, repo, "tickets", "root")
	if err != nil {
		t.Fatalf("execute tickets root: %v", err)
	}
	if want := project + "\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestExecute_TicketsRoot_HonorsConfiguredStorePath(t *testing.T) {
	cmdtest.IsolateTicketStore(t)
	repo := testutil.TempRepo(t)
	custom := filepath.Join(t.TempDir(), "custom-store")
	project := cmdtest.AddProject(t, custom, "mine", repo)
	cfgDir := filepath.Join(cmdtest.EnvValue(cmdtest.EnvFor(t), "HOME"), ".config", "gx")
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
	store := cmdtest.IsolateTicketStore(t)
	repo := testutil.TempRepo(t)
	cmdtest.AddProject(t, store, "other", filepath.Join(t.TempDir(), "elsewhere"))

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
	d := testDeps(t, &stdout, &stderr, func() (string, error) { return dir, nil })

	err := execute(t, []string{"tickets", "root"}, d)
	if err == nil {
		t.Fatal("expected error when cwd is outside a git repo, got nil")
	}
	if !strings.Contains(err.Error(), "not inside a git repo") {
		t.Errorf("error = %q, want it to mention not being inside a git repo", err.Error())
	}
}
