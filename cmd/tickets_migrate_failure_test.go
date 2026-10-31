package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migratefail"
)

const cyclicParents = "---\nid: \"01\"\nstatus: open\ntype: implement\nparent: \"02\"\n---\nBody.\n"

func seedFailure(t *testing.T, dirs config.Dirs, project string) string {
	t.Helper()
	state, err := dirs.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := migratefail.Record(state, project, "earlier failure"); err != nil {
		t.Fatal(err)
	}
	return state
}

func recordedFor(t *testing.T, state, project string) bool {
	t.Helper()
	got, err := migratefail.Read(state, []string{project})
	if err != nil {
		t.Fatal(err)
	}
	return len(got) == 1
}

func TestMigrate_PassingRunClearsTheProjectsFailure(t *testing.T) {
	t.Parallel()
	dirs := config.DirsFrom(tempEnv(t))
	dir := writeStoreProject(t, t.TempDir(), "alpha", map[string]string{"01-a.md": legacyStatusTicket})
	state := seedFailure(t, dirs, "alpha")
	_ = migratefail.Record(state, "beta", "other")

	if err := runTicketsMigrate(dirs, dir, false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if recordedFor(t, state, "alpha") {
		t.Error("alpha's failure survived a passing migration")
	}
	if !recordedFor(t, state, "beta") {
		t.Error("beta's failure was cleared by alpha's migration")
	}
}

func TestMigrate_FailingRunLeavesTheRecordUnchanged(t *testing.T) {
	t.Parallel()
	dirs := config.DirsFrom(tempEnv(t))
	dir := writeStoreProject(t, t.TempDir(), "alpha", map[string]string{
		"01-a.md": cyclicParents,
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\nBody.\n",
	})
	state := seedFailure(t, dirs, "alpha")
	before, err := os.ReadFile(migratefail.Path(state))
	if err != nil {
		t.Fatal(err)
	}

	if err := runTicketsMigrate(dirs, dir, false, &bytes.Buffer{}); err == nil {
		t.Fatal("want a migration error")
	}
	after, err := os.ReadFile(migratefail.Path(state))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("record changed:\nbefore %s\nafter  %s", before, after)
	}
}

func TestMigrate_DryRunKeepsTheFailure(t *testing.T) {
	t.Parallel()
	dirs := config.DirsFrom(tempEnv(t))
	dir := writeStoreProject(t, t.TempDir(), "alpha", map[string]string{"01-a.md": legacyStatusTicket})
	state := seedFailure(t, dirs, "alpha")

	if err := runTicketsMigrate(dirs, dir, true, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !recordedFor(t, state, "alpha") {
		t.Error("a dry run cleared the failure")
	}
}
