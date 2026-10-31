package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migratefail"
)

// writeStoreProject makes <store>/<name> a project holding one epic with the
// given tickets.
func writeStoreProject(t *testing.T, store, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(store, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	writeMigrateFixtureEpic(t, dir, "epic", files)
	return dir
}

const legacyNeedsAnswer = "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\n---\nBody.\n"

func TestMigrateAllProjects_MigratesEveryStoreAndNamesEach(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	a := writeStoreProject(t, store, "alpha", map[string]string{"01-a.md": legacyNeedsAnswer})
	b := writeStoreProject(t, store, "beta", map[string]string{"01-a.md": legacyNeedsAnswer})

	var out bytes.Buffer
	if err := migrateAllProjects(config.DirsFrom(tempEnv(t)), store, &out); err != nil {
		t.Fatalf("migrateAllProjects: %v", err)
	}

	for _, dir := range []string{a, b} {
		got, err := os.ReadFile(filepath.Join(dir, "epic", "issues", "01-a.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), "status: paused") {
			t.Errorf("%s not migrated to paused:\n%s", dir, got)
		}
	}
	for _, want := range []string{"alpha", "beta"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not name %q:\n%s", want, out.String())
		}
	}
}

func TestMigrateAllProjects_FailureNamesTheStore(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	writeStoreProject(t, store, "broken", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\nparent: \"02\"\n---\nBody.\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\nBody.\n",
	})

	err := migrateAllProjects(config.DirsFrom(tempEnv(t)), store, &bytes.Buffer{})
	if err == nil {
		t.Fatal("want an error for a store that cannot migrate")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error does not name the failed store: %v", err)
	}
}

func TestMigrateAllProjectsContinue_RecordsFailureAndMigratesTheRest(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	state := t.TempDir()
	writeStoreProject(t, store, "alpha", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\nparent: \"02\"\n---\nBody.\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\nBody.\n",
	})
	good := writeStoreProject(t, store, "beta", map[string]string{"01-a.md": legacyNeedsAnswer})

	var out bytes.Buffer
	if err := migrateAllProjectsContinue(config.DirsFrom(tempEnv(t)), state, store, &out); err != nil {
		t.Fatalf("want nil after a failed project, got %v", err)
	}

	got, err := os.ReadFile(filepath.Join(good, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "status: paused") {
		t.Errorf("beta not migrated after alpha failed:\n%s", got)
	}
	fails, err := migratefail.Read(state, []string{"alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fails) != 1 || fails[0].Project != "alpha" || fails[0].Error == "" {
		t.Errorf("failures = %+v, want one for alpha", fails)
	}
}

func TestMigrateAllProjectsContinue_RecordWriteFailureDoesNotStopLaterProjects(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	// A regular file as the state dir makes every migratefail.Record fail.
	state := filepath.Join(t.TempDir(), "state-is-a-file")
	if err := os.WriteFile(state, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeStoreProject(t, store, "alpha", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\nparent: \"02\"\n---\nBody.\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\nBody.\n",
	})
	good := writeStoreProject(t, store, "beta", map[string]string{"01-a.md": legacyNeedsAnswer})

	var out bytes.Buffer
	if err := migrateAllProjectsContinue(config.DirsFrom(tempEnv(t)), state, store, &out); err != nil {
		t.Fatalf("want nil when the record cannot be saved, got %v", err)
	}

	got, err := os.ReadFile(filepath.Join(good, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "status: paused") {
		t.Errorf("beta not migrated after the record write failed:\n%s", got)
	}
}

func TestMigrateAllProjectsContinue_NamesProjectByProjectJSON(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	state := t.TempDir()
	dir := writeStoreProject(t, store, "alpha-dir", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\nparent: \"02\"\n---\nBody.\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\nBody.\n",
	})
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(`{"name":"pretty"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := migrateAllProjectsContinue(config.DirsFrom(tempEnv(t)), state, store, &out); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "migrating pretty\n") || !strings.Contains(out.String(), "migrate pretty failed:") {
		t.Errorf("progress lines do not use the project.json name:\n%s", out.String())
	}
}

func TestRestartLogPath_IsFixedFileInStateDir(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	if got, want := migratefail.RestartLogPath(state), filepath.Join(state, "restart.log"); got != want {
		t.Errorf("RestartLogPath = %s, want %s", got, want)
	}
}

func TestRestartSteps_MigrateFailureDoesNotStartServer(t *testing.T) {
	t.Parallel()
	var calls []string
	step := func(name string, err error) func() error {
		return func() error { calls = append(calls, name); return err }
	}

	err := restartSteps(true, step("stop", nil), step("migrate", errors.New("boom")), step("start", nil))
	if err == nil {
		t.Fatal("want the migrate error")
	}
	if got := strings.Join(calls, ","); got != "stop,migrate" {
		t.Errorf("calls = %s, want stop,migrate", got)
	}
}

func TestRestartSteps_OrderAndMigrateOptional(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		migrate bool
		want    string
	}{
		{true, "stop,migrate,start"},
		{false, "stop,start"},
	} {
		var calls []string
		step := func(name string) func() error {
			return func() error { calls = append(calls, name); return nil }
		}
		if err := restartSteps(tc.migrate, step("stop"), step("migrate"), step("start")); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(calls, ","); got != tc.want {
			t.Errorf("migrate=%v: calls = %s, want %s", tc.migrate, got, tc.want)
		}
	}
}
