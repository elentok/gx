package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
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
