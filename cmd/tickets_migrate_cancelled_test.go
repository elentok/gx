package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

func cancelledTicket(body string) string {
	return strings.Replace(creationTicket(body), "status: open", "status: cancelled", 1)
}

func TestExecute_TicketsMigrate_BackfillsCancelledAtFromStoreHistory(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	testutil.MustGitExported(t, store, "init", "-q")
	testutil.MustGitExported(t, store, "config", "user.email", "test@test.com")
	testutil.MustGitExported(t, store, "config", "user.name", "Test")
	project := filepath.Join(store, "p")
	testutil.Mkdir(t, project)
	testutil.WriteFile(t, project, "project.json", "{}")

	stamped := "---\nid: \"03\"\nstatus: cancelled\ntype: implement\ncancelled_at: 2020-01-01T00:00:00Z\n---\nBody.\n"
	writeMigrateFixtureEpic(t, project, "e", map[string]string{
		"01-later.md":   creationTicket("Cancelled later."),
		"02-open.md":    creationTicket("Still open."),
		"03-stamped.md": stamped,
	})
	writeMigrateFixtureEpic(t, project, "follow-ups", map[string]string{"01-fu.md": creationTicket("A follow-up.")})
	commitStoreAt(t, store, "import", "2024-03-05T13:45:00Z")

	ticketPath := func(parts ...string) string { return filepath.Join(append([]string{project}, parts...)...) }
	if err := os.WriteFile(ticketPath("e", "issues", "01-later.md"), []byte(cancelledTicket("Cancelled later.")), 0o644); err != nil {
		t.Fatal(err)
	}
	commitStoreAt(t, store, "cancel", "2025-06-07T08:09:10Z")

	// Cancelled and closed into the archive epic in one commit.
	if err := os.WriteFile(ticketPath("follow-ups", "issues", "01-fu.md"), []byte(cancelledTicket("A follow-up.")), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Mkdir(t, ticketPath("follow-ups-archived"))
	testutil.MustGitExported(t, store, "mv", "p/follow-ups/issues", "p/follow-ups-archived/issues")
	commitStoreAt(t, store, "close follow-up", "2026-01-02T03:04:05Z")

	run := func(args ...string) string {
		t.Helper()
		var stdout bytes.Buffer
		argv := append([]string{"tickets", "migrate", project}, args...)
		if err := execute(argv, deps{stdout: &stdout, stderr: &bytes.Buffer{}}); err != nil {
			t.Fatalf("execute %v: %v", argv, err)
		}
		return stdout.String()
	}
	cancelledAt := func(rel string) time.Time {
		t.Helper()
		tk, err := schema.ParseTicket(ticketPath(rel))
		if err != nil {
			t.Fatal(err)
		}
		return tk.CancelledAt
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
	var listed bool
	for line := range strings.SplitSeq(dry, "\n") {
		listed = listed || strings.Contains(line, "p/e/issues/01-later.md:") && strings.Contains(line, "cancelled_at backfilled")
	}
	if !listed {
		t.Errorf("dry run does not list the cancelled ticket:\n%s", dry)
	}
	if strings.Contains(dry, "02-open.md: cancelled_at") {
		t.Errorf("dry run lists an open ticket for cancelled_at:\n%s", dry)
	}
	if !cancelledAt("e/issues/01-later.md").IsZero() {
		t.Error("dry run wrote cancelled_at")
	}

	run()

	cases := map[string]time.Time{
		"e/issues/01-later.md":                utc("2025-06-07T08:09:10Z"),
		"e/issues/02-open.md":                 {},
		"e/issues/03-stamped.md":              utc("2020-01-01T00:00:00Z"),
		"follow-ups-archived/issues/01-fu.md": utc("2026-01-02T03:04:05Z"),
	}
	for rel, want := range cases {
		if got := cancelledAt(rel); !got.Equal(want) {
			t.Errorf("%s: cancelled_at %v, want %v", rel, got, want)
		}
	}

	if again := run(); !strings.Contains(again, "no changes") {
		t.Errorf("second run changed something:\n%s", again)
	}
}
