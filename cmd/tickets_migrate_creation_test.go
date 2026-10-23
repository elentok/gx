package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// creationTicket needs a distinct body per ticket: git log --follow treats
// identical files as copies of each other.
func creationTicket(body string) string {
	return "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n" + strings.Repeat(body+"\n", 20)
}

func commitStoreAt(t *testing.T, store, message, when string) {
	t.Helper()
	testutil.MustGitExported(t, store, "add", ".")
	c := exec.Command("git", "commit", "-m", message)
	c.Dir = store
	c.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+when, "GIT_AUTHOR_DATE="+when)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("commit %q: %v\n%s", message, err, out)
	}
}

func TestExecute_TicketsMigrate_BackfillsCreationFromStoreHistory(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	testutil.MustGitExported(t, store, "init", "-q")
	testutil.MustGitExported(t, store, "config", "user.email", "test@test.com")
	testutil.MustGitExported(t, store, "config", "user.name", "Test")
	project := filepath.Join(store, "p")
	testutil.Mkdir(t, project)
	testutil.WriteFile(t, project, "project.json", "{}")

	// The import commit: pre-store tickets, one of them archived.
	writeMigrateFixtureEpic(t, project, "imported", map[string]string{"01-old.md": creationTicket("Imported.")})
	writeMigrateFixtureEpic(t, filepath.Join(project, ".archive"), "shelved", map[string]string{"01-old.md": creationTicket("Shelved.")})
	commitStoreAt(t, store, "import", "2024-03-05T13:45:00Z")

	writeMigrateFixtureEpic(t, project, "first", map[string]string{
		"01-new.md":  creationTicket("Brand new."),
		"02-kept.md": "---\nid: \"02\"\nstatus: open\ntype: implement\ncreated_at: 2020-01-01T00:00:00Z\noriginal_address: \"p:elsewhere/09\"\n---\nBody.\n",
	})
	commitStoreAt(t, store, "add tickets", "2025-06-07T08:09:10Z")
	testutil.MustGitExported(t, store, "mv", "p/first", "p/second")
	commitStoreAt(t, store, "move epic", "2026-01-02T03:04:05Z")

	run := func(args ...string) string {
		t.Helper()
		var stdout bytes.Buffer
		argv := append([]string{"tickets", "migrate", project}, args...)
		if err := execute(argv, deps{stdout: &stdout, stderr: &bytes.Buffer{}}); err != nil {
			t.Fatalf("execute %v: %v", argv, err)
		}
		return stdout.String()
	}
	load := func(rel string) schema.Ticket {
		t.Helper()
		tk, err := schema.ParseTicket(filepath.Join(project, rel))
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}
	utc := func(s string) time.Time {
		t.Helper()
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	dry := run("--dry-run")
	if !strings.Contains(dry, "p/second/issues/01-new.md: created_at backfilled") {
		t.Errorf("dry run does not list the new ticket:\n%s", dry)
	}
	if !load("second/issues/01-new.md").CreatedAt.IsZero() {
		t.Error("dry run wrote created_at")
	}

	run()

	cases := []struct {
		rel      string
		created  time.Time
		original string
	}{
		{"imported/issues/01-old.md", utc("2024-03-05T00:00:00Z"), "p:imported/01"},
		{".archive/shelved/issues/01-old.md", utc("2024-03-05T00:00:00Z"), "p:shelved/01"},
		{"second/issues/01-new.md", utc("2025-06-07T08:09:10Z"), "p:first/01"},
		{"second/issues/02-kept.md", utc("2020-01-01T00:00:00Z"), "p:elsewhere/09"},
	}
	for _, c := range cases {
		got := load(c.rel)
		if !got.CreatedAt.Equal(c.created) || got.OriginalAddress != c.original {
			t.Errorf("%s: created_at %v, original %q; want %v, %q", c.rel, got.CreatedAt, got.OriginalAddress, c.created, c.original)
		}
	}

	if again := run(); !strings.Contains(again, "no changes") {
		t.Errorf("second run changed something:\n%s", again)
	}
}
