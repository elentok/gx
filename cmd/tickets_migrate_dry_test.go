package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
)

func dryRunMigrate(t *testing.T, root string) (string, error) {
	t.Helper()
	var w bytes.Buffer
	err := runTicketsMigrate(config.DirsFrom(envFor(t)), root, true, &w)
	return w.String(), err
}

func assertTreeUnchanged(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := snapshotTree(t, root)
	if len(after) != len(before) {
		t.Fatalf("dry run changed the file set: before %d files, after %d", len(before), len(after))
	}
	for p, b := range before {
		if after[p] != b {
			t.Errorf("dry run changed %s", p)
		}
	}
}

func TestRunTicketsMigrate_DryRunFailsOnInvalidFullRewrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMigrateFixtureEpic(t, root, "widget-epic", map[string]string{
		"01-bad.md": "---\nid: \"01\"\nstatus: open\ntype: not-a-type\n---\nBody.\n",
	})
	before := snapshotTree(t, root)

	out, err := dryRunMigrate(t, root)
	if err == nil {
		t.Fatalf("dry run passed an invalid ticket:\n%s", out)
	}
	if !strings.Contains(err.Error(), filepath.Join(root, "widget-epic", "issues", "01-bad.md")) {
		t.Errorf("error does not name the real ticket path: %v", err)
	}
	assertTreeUnchanged(t, root, before)
}

func TestRunTicketsMigrate_DryRunCatchesFailureOnPreviousStepsOutput(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Each ticket parses on its own; only the migrated epic's parent graph
	// (checked after the rewrite step) is invalid.
	writeMigrateFixtureEpic(t, root, "widget-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\nparent: \"02\"\n---\nBody.\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\nBody.\n",
	})
	before := snapshotTree(t, root)

	if out, err := dryRunMigrate(t, root); err == nil {
		t.Fatalf("dry run passed a parent cycle:\n%s", out)
	}
	assertTreeUnchanged(t, root, before)
}

func TestRunTicketsMigrate_DryRunPassingProjectListsChangesAndWritesNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeMigrateFixtureEpic(t, root, "widget-epic", map[string]string{
		"01-old.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\n---\nBody.\n",
	})
	before := snapshotTree(t, root)

	out, err := dryRunMigrate(t, root)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(out, filepath.Join(root, "widget-epic", "issues", "01-old.md")+": ") ||
		!strings.Contains(out, "file(s) would change") {
		t.Errorf("dry run output lacks the would-change list:\n%s", out)
	}
	assertTreeUnchanged(t, root, before)
}
