package serverctl

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migrations"
)

func writeRestartCheckConfig(t *testing.T, dirs config.Dirs, body string) {
	t.Helper()
	path, err := dirs.FilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runRestartCheckJSON(t *testing.T, dirs config.Dirs) restartCheck {
	t.Helper()
	var out bytes.Buffer
	if err := runRestartCheck(dirs, false, &out); err != nil {
		t.Fatalf("runRestartCheck: %v", err)
	}
	var got restartCheck
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	return got
}

func TestRestartCheck_ValidConfigReportsVersionConfigAndProjects(t *testing.T) {
	t.Parallel()
	dirs := config.DirsFrom(cmdtest.TempEnv(t))
	store := t.TempDir()
	cmdtest.WriteStoreProject(t, store, "alpha", map[string]string{"01-a.md": legacyStatusTicket})
	writeRestartCheckConfig(t, dirs, `{"ticket-store": {"path": "`+store+`"}}`)

	got := runRestartCheckJSON(t, dirs)

	if got.Version != getVersion() {
		t.Errorf("version = %q, want %q", got.Version, getVersion())
	}
	if !got.Config.OK {
		t.Errorf("config not ok: %s", got.Config.Error)
	}
	if len(got.Projects) != 1 || got.Projects[0].Name != "alpha" || !got.Projects[0].OK {
		t.Fatalf("projects = %+v, want alpha passing", got.Projects)
	}
	if len(got.Projects[0].Files) == 0 {
		t.Errorf("alpha lists no would-change files")
	}
}

func TestRestartCheck_NamesProjectByProjectJSON(t *testing.T) {
	t.Parallel()
	dirs := config.DirsFrom(cmdtest.TempEnv(t))
	store := t.TempDir()
	dir := cmdtest.WriteStoreProject(t, store, "alpha-dir", map[string]string{"01-a.md": legacyStatusTicket})
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(`{"name":"pretty"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRestartCheckConfig(t, dirs, `{"ticket-store": {"path": "`+store+`"}}`)

	got := runRestartCheckJSON(t, dirs)

	if len(got.Projects) != 1 || got.Projects[0].Name != "pretty" {
		t.Errorf("projects = %+v, want one named pretty", got.Projects)
	}
}

func TestRestartCheck_BrokenConfigReportsStartupErrorText(t *testing.T) {
	t.Parallel()
	dirs := config.DirsFrom(cmdtest.TempEnv(t))
	writeRestartCheckConfig(t, dirs, `{not json`)
	_, wantErr := dirs.Load()
	if wantErr == nil {
		t.Fatal("fixture config loads")
	}

	got := runRestartCheckJSON(t, dirs)

	if got.Config.OK || got.Config.Error != wantErr.Error() {
		t.Errorf("config = %+v, want failure %q", got.Config, wantErr)
	}
	var out bytes.Buffer
	err := runRestartCheck(dirs, true, &out)
	if err == nil || err.Error() != wantErr.Error() || out.Len() != 0 {
		t.Errorf("config-only = %v, out %q; want the startup error and no output", err, out.String())
	}
}

func TestRestartCheck_DryRunsOnlyThePendingSteps(t *testing.T) {
	t.Parallel()
	dirs := config.DirsFrom(cmdtest.TempEnv(t))
	store := t.TempDir()
	full := cmdtest.WriteStoreProject(t, store, "full", map[string]string{"01-a.md": legacyStatusTicket})
	if err := migrations.WriteFullApplied(full); err != nil {
		t.Fatal(err)
	}
	cmdtest.WriteStoreProject(t, store, "nolist", map[string]string{"01-a.md": legacyStatusTicket})
	writeRestartCheckConfig(t, dirs, `{"ticket-store": {"path": "`+store+`"}}`)
	before := cmdtest.SnapshotTree(t, full)

	got := runRestartCheckJSON(t, dirs)

	byName := map[string]restartCheckProject{}
	for _, p := range got.Projects {
		byName[p.Name] = p
	}
	if p := byName["full"]; !p.OK || len(p.Files) != 0 || p.Pending != 0 {
		t.Errorf("full = %+v, want ok, no files, 0 pending", p)
	}
	if p := byName["nolist"]; !p.OK || len(p.Files) == 0 || p.Pending != len(migrations.Default.Names()) {
		t.Errorf("nolist = %+v, want the whole chain pending and files reported", p)
	}
	cmdtest.AssertTreeUnchanged(t, full, before)
}

func TestRestartCheck_FailingMigrationShowsAsFailedAndWritesNothing(t *testing.T) {
	t.Parallel()
	dirs := config.DirsFrom(cmdtest.TempEnv(t))
	store := t.TempDir()
	dir := cmdtest.WriteStoreProject(t, store, "broken", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\nparent: \"02\"\n---\nBody.\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\nBody.\n",
	})
	writeRestartCheckConfig(t, dirs, `{"ticket-store": {"path": "`+store+`"}}`)
	before := cmdtest.SnapshotTree(t, dir)

	got := runRestartCheckJSON(t, dirs)

	if len(got.Projects) != 1 || got.Projects[0].OK || !strings.Contains(got.Projects[0].Error, "broken") {
		t.Errorf("projects = %+v, want a failed dry run naming the project path", got.Projects)
	}
	cmdtest.AssertTreeUnchanged(t, dir, before)
}
