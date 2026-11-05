package ticketsmigrate

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/config"
)

func dryRunMigrate(t *testing.T, root string) (string, error) {
	t.Helper()
	var w bytes.Buffer
	err := runTicketsMigrate(config.DirsFrom(cmdtest.EnvFor(t)), root, true, &w)
	return w.String(), err
}

func TestRunTicketsMigrate_DryRunFailsOnInvalidFullRewrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cmdtest.WriteMigrateFixtureEpic(t, root, "widget-epic", map[string]string{
		"01-bad.md": "---\nid: \"01\"\nstatus: open\ntype: not-a-type\n---\nBody.\n",
	})
	before := cmdtest.SnapshotTree(t, root)

	out, err := dryRunMigrate(t, root)
	if err == nil {
		t.Fatalf("dry run passed an invalid ticket:\n%s", out)
	}
	if !strings.Contains(err.Error(), filepath.Join(root, "widget-epic", "issues", "01-bad.md")) {
		t.Errorf("error does not name the real ticket path: %v", err)
	}
	cmdtest.AssertTreeUnchanged(t, root, before)
}

func TestRunTicketsMigrate_DryRunCatchesFailureOnPreviousStepsOutput(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Each ticket parses on its own; only the migrated epic's parent graph
	// (checked after the rewrite step) is invalid.
	cmdtest.WriteMigrateFixtureEpic(t, root, "widget-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\nparent: \"02\"\n---\nBody.\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nparent: \"01\"\n---\nBody.\n",
	})
	before := cmdtest.SnapshotTree(t, root)

	if out, err := dryRunMigrate(t, root); err == nil {
		t.Fatalf("dry run passed a parent cycle:\n%s", out)
	}
	cmdtest.AssertTreeUnchanged(t, root, before)
}

func TestRunTicketsMigrate_DryRunPassingProjectListsChangesAndWritesNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cmdtest.WriteMigrateFixtureEpic(t, root, "widget-epic", map[string]string{
		"01-old.md": cmdtest.LegacyStatusTicket,
	})
	before := cmdtest.SnapshotTree(t, root)

	out, err := dryRunMigrate(t, root)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(out, filepath.Join(root, "widget-epic", "issues", "01-old.md")+": ") ||
		!strings.Contains(out, "file(s) would change") {
		t.Errorf("dry run output lacks the would-change list:\n%s", out)
	}
	cmdtest.AssertTreeUnchanged(t, root, before)
}
