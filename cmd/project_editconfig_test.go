package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/testutil"
)

const editConfigOrig = `{"name": "proj"}`

type editConfigEnv struct {
	store, live string
	out, errOut bytes.Buffer
	d           deps
	edits       int
	lastTmp     string
}

// newEditConfigEnv registers project "proj" for repo and scripts the editor:
// each run writes the next of saves into the temp copy.
func newEditConfigEnv(t *testing.T, tty bool, answers string, saves ...string) *editConfigEnv {
	t.Helper()
	e := &editConfigEnv{store: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(e.store, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.live = filepath.Join(e.store, "proj", config.ProjectFileName)
	if err := os.WriteFile(e.live, []byte(editConfigOrig), 0o644); err != nil {
		t.Fatal(err)
	}
	e.d = deps{
		stdin:           strings.NewReader(answers),
		stdout:          &e.out,
		stderr:          &e.errOut,
		getwd:           func() (string, error) { return t.TempDir(), nil },
		getenv:          func(string) string { return "vi" },
		loadConfig:      func() (config.Config, error) { return config.Config{TicketStore: config.TicketStoreConfig{Path: e.store}}, nil },
		stdinIsTerminal: func() bool { return tty },
		runEditor: func(_, path string, _ io.Reader, _, _ io.Writer) error {
			if e.edits >= len(saves) {
				t.Fatalf("editor opened %d times, scripted %d", e.edits+1, len(saves))
			}
			e.lastTmp = path
			e.edits++
			return os.WriteFile(path, []byte(saves[e.edits-1]), 0o600)
		},
	}
	return e
}

func (e *editConfigEnv) liveBody(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(e.live)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestProjectEditConfig_ValidEditWritten(t *testing.T) {
	saved := `{"name": "proj", "trunk": "develop", "landing": "on-done"}`
	e := newEditConfigEnv(t, false, "", saved)
	if err := runProjectEditConfig(e.d, "proj"); err != nil {
		t.Fatal(err)
	}
	if got := e.liveBody(t); got != saved {
		t.Errorf("live = %s, want %s", got, saved)
	}
	if _, err := config.ReadProjectFile(filepath.Dir(e.live)); err != nil {
		t.Errorf("server loader rejects the saved file: %v", err)
	}
	if _, err := os.Stat(e.lastTmp); !os.IsNotExist(err) {
		t.Errorf("temp copy left behind: %v", err)
	}
}

func TestProjectEditConfig_EmptyTrunkSaves(t *testing.T) {
	saved := `{"name": "proj", "trunk": ""}`
	e := newEditConfigEnv(t, false, "", saved)
	if err := runProjectEditConfig(e.d, "proj"); err != nil {
		t.Fatal(err)
	}
	if got := e.liveBody(t); got != saved {
		t.Errorf("live = %s, want %s", got, saved)
	}
}

func TestProjectEditConfig_NoChanges(t *testing.T) {
	e := newEditConfigEnv(t, false, "", editConfigOrig)
	if err := runProjectEditConfig(e.d, "proj"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.out.String(), "no changes") {
		t.Errorf("out = %q, want no changes", e.out.String())
	}
}

func TestProjectEditConfig_InvalidSaveLoops(t *testing.T) {
	invalid := map[string]string{
		"key":   `{"name": "proj", "bogus": 1}`,
		"type":  `{"name": "proj", "max-agents": "two"}`,
		"landing": `{"name": "proj", "landing": "pr"}`,
		"vcs":     `{"name": "proj", "vcs": "hg"}`,
		"trunk":   `{"name": "proj", "trunk": "bad branch.."}`,
	}
	for kind, bad := range invalid {
		t.Run(kind+"/yes", func(t *testing.T) {
			fixed := `{"name": "proj", "trunk": "develop"}`
			e := newEditConfigEnv(t, true, "\n", bad, fixed)
			if err := runProjectEditConfig(e.d, "proj"); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(e.out.String(), "Edit again? [Y/n]") {
				t.Errorf("no prompt in %q", e.out.String())
			}
			if got := e.liveBody(t); got != fixed {
				t.Errorf("live = %s, want %s", got, fixed)
			}
		})
		t.Run(kind+"/no", func(t *testing.T) {
			e := newEditConfigEnv(t, true, "n\n", bad)
			if err := runProjectEditConfig(e.d, "proj"); err != nil {
				t.Fatal(err)
			}
			if got := e.liveBody(t); got != editConfigOrig {
				t.Errorf("live touched: %s", got)
			}
		})
	}
}

func TestProjectEditConfig_NonTTYInvalidFailsWithoutPrompt(t *testing.T) {
	e := newEditConfigEnv(t, false, "y\n", `{"bogus": 1}`)
	err := runProjectEditConfig(e.d, "proj")
	if err == nil || !strings.Contains(err.Error(), e.lastTmp) {
		t.Fatalf("err = %v, want error naming %s", err, e.lastTmp)
	}
	if _, err := os.Stat(e.lastTmp); err != nil {
		t.Errorf("temp copy not kept: %v", err)
	}
	os.Remove(e.lastTmp)
	if strings.Contains(e.out.String(), "Edit again") {
		t.Errorf("prompted without a TTY: %q", e.out.String())
	}
	if got := e.liveBody(t); got != editConfigOrig {
		t.Errorf("live touched: %s", got)
	}
}

func TestProjectEditConfig_ConcurrentChangeRefusedKeepsTemp(t *testing.T) {
	e := newEditConfigEnv(t, false, "")
	concurrent := `{"name": "proj", "repo": "/elsewhere"}`
	e.d.runEditor = func(_, path string, _ io.Reader, _, _ io.Writer) error {
		e.lastTmp = path
		if err := os.WriteFile(e.live, []byte(concurrent), 0o644); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(`{"name": "proj", "trunk": "develop"}`), 0o600)
	}
	err := runProjectEditConfig(e.d, "proj")
	if err == nil || !strings.Contains(err.Error(), "changed while you were editing") || !strings.Contains(err.Error(), e.lastTmp) {
		t.Fatalf("err = %v, want refusal naming %s", err, e.lastTmp)
	}
	if got := e.liveBody(t); got != concurrent {
		t.Errorf("concurrent change overwritten: %s", got)
	}
	if _, err := os.Stat(e.lastTmp); err != nil {
		t.Errorf("temp copy not kept: %v", err)
	}
	os.Remove(e.lastTmp)
}

func TestProjectEditConfig_UnregisteredRepoNeverCreatesProjectJSON(t *testing.T) {
	e := newEditConfigEnv(t, false, "")
	repo := testutil.TempRepo(t)
	e.d.getwd = func() (string, error) { return repo, nil }
	if err := runProjectEditConfig(e.d, ""); !errors.Is(err, tickets.ErrNoProject) {
		t.Errorf("err = %v, want ErrNoProject", err)
	}
	if err := runProjectEditConfig(e.d, "nope"); err == nil {
		t.Error("unknown name: want error")
	}
	if _, err := os.Stat(filepath.Join(e.store, "nope")); !os.IsNotExist(err) {
		t.Errorf("project dir created: %v", err)
	}
}

// newInFlightEnv gives proj a repo on trunk main and four roots: one with a
// claimed ticket and a claimed child, one with an unlanded feature branch, one
// whose branch is on main, and one never started.
func newInFlightEnv(t *testing.T, saved string) *editConfigEnv {
	t.Helper()
	repo := testutil.TempRepo(t)
	e := newEditConfigEnv(t, false, "", strings.ReplaceAll(saved, "REPO", repo))
	testutil.WriteFile(t, filepath.Dir(e.live), config.ProjectFileName, `{"name": "proj", "repo": "`+repo+`", "trunk": "main"}`)
	testutil.MustGitExported(t, repo, "branch", "landed")
	testutil.MustGitExported(t, repo, "switch", "-qc", "unlanded")
	testutil.MustGitExported(t, repo, "commit", "-q", "--allow-empty", "-m", "wip")
	testutil.MustGitExported(t, repo, "switch", "-q", "main")
	roots := map[string]string{
		"claimed":  "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# a\n",
		"unlanded": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# a\n",
		"landed":   "---\nid: \"01\"\nstatus: done\ntype: implement\n---\n# a\n",
		"idle":     "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# a\n",
	}
	for epic, body := range roots {
		dir := filepath.Join(e.store, "proj", epic)
		testutil.EnsureEpicTicketMD(t, dir)
		testutil.Mkdir(t, filepath.Join(dir, "issues"))
		testutil.WriteFile(t, filepath.Join(dir, "issues"), "01-a.md", body)
	}
	testutil.WriteFile(t, filepath.Join(e.store, "proj", "claimed", "issues"), "01a-child.md",
		"---\nid: 01a\nparent: \"01\"\nstatus: claimed\ntype: implement\n---\n# child\n")
	return e
}

func TestProjectEditConfig_TrunkChangeWarnsRootsInFlight(t *testing.T) {
	e := newInFlightEnv(t, `{"name": "proj", "repo": "REPO", "trunk": "develop"}`)
	if err := runProjectEditConfig(e.d, "proj"); err != nil {
		t.Fatal(err)
	}
	if want := "2 tickets in flight will land on develop\n"; !strings.HasSuffix(e.out.String(), want) {
		t.Errorf("out = %q, want suffix %q", e.out.String(), want)
	}
}

func TestProjectEditConfig_UnchangedTrunkNoWarning(t *testing.T) {
	e := newInFlightEnv(t, `{"name": "proj", "repo": "REPO", "trunk": "main", "landing": "on-done"}`)
	if err := runProjectEditConfig(e.d, "proj"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.out.String(), "in flight") {
		t.Errorf("warned without a trunk change: %q", e.out.String())
	}
}

func TestProjectEditConfig_CwdProject(t *testing.T) {
	e := newEditConfigEnv(t, false, "", `{"name": "proj", "repo": "REPO", "trunk": "develop"}`)
	repo := testutil.TempRepo(t)
	body := `{"name": "proj", "repo": "` + repo + `"}`
	if err := os.WriteFile(e.live, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	e.d.getwd = func() (string, error) { return repo, nil }
	if err := runProjectEditConfig(e.d, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.liveBody(t), "develop") {
		t.Errorf("cwd project not edited: %s", e.liveBody(t))
	}
}
