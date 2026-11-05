package ticketsmigrate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// importOldTicket writes one old-shape ticket (plus an optional event) into a
// fresh old tree, imports it with --to-store as project "mine" and returns the
// project directory and the imported ticket path.
func importOldTicket(t *testing.T, store, status, body string, ev *tickets.LogEvent) (project, ticket string) {
	t.Helper()
	repo := testutil.TempRepo(t)
	old := filepath.Join(t.TempDir(), ".scratch")
	issues := filepath.Join(old, "widget", "issues")
	if err := os.MkdirAll(issues, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(issues, "01-first.md")
	raw := "---\nid: \"01\"\nstatus: " + status + "\ntype: task\n---\n" + body
	if err := os.WriteFile(src, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if ev != nil {
		if _, err := tickets.AppendLog(src, *ev); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runIn(t, repo, "tickets", "migrate", "--to-store", "--project", "mine", old); err != nil {
		t.Fatalf("--to-store: %v", err)
	}
	project = filepath.Join(store, "mine")
	return project, filepath.Join(project, "widget", "issues", "01-first.md")
}

// serverStart is the migration run of a server start (see server.migrateProject).
func serverStart(t *testing.T, project string) ([]tickets.MigrationChange, error) {
	t.Helper()
	changes, _, err := migrations.Run(migrations.Env{
		Dirs: config.DirsFrom(cmdtest.EnvFor(t)), Root: project, LookupRoot: project,
		AppliedPath: filepath.Join(project, migrations.AppliedFile),
		SkipApplied: true,
	})
	return changes, err
}

func appliedNames(t *testing.T, project string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(project, migrations.AppliedFile))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(raw))
}

func readPaused(t *testing.T, path string) schema.Ticket {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := schema.ParseTicketFromRawUnshimmed(string(raw), path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestExecute_TicketsMigrateToStore_FirstServerStartRunsEveryStep(t *testing.T) {
	store := cmdtest.IsolateTicketStore(t)
	project, _ := importOldTicket(t, store, "open", "Body.\n", nil)

	if _, err := serverStart(t, project); err != nil {
		t.Fatalf("server start: %v", err)
	}
	if got, want := appliedNames(t, project), migrations.Default.Names(); !slices.Equal(got, want) {
		t.Errorf("applied after the first start = %v, want every step %v", got, want)
	}
}

func TestExecute_TicketsMigrateToStore_NeedsRepairKeepsItsPauseReasonAfterStart(t *testing.T) {
	store := cmdtest.IsolateTicketStore(t)
	ev := tickets.LogEvent{Type: tickets.EventPauseQuestion, By: "gx", PauseKind: "spinning", Summary: "stuck"}
	project, ticket := importOldTicket(t, store, "needs-repair", "Body.\n", &ev)

	if _, err := serverStart(t, project); err != nil {
		t.Fatalf("server start: %v", err)
	}
	got := readPaused(t, ticket)
	if got.Status != schema.StatusPaused || got.PauseReason != schema.PauseSpinning {
		t.Errorf("status %q reason %q, want paused %q", got.Status, got.PauseReason, schema.PauseSpinning)
	}
}

// A run that stops after the frontmatter migration of the pause unit leaves the
// unit unrecorded; the retry reruns all of it and the reason is still the one
// from the event log, although the first half already rewrote the ticket.
func TestExecute_TicketsMigrateToStore_PartialPauseUnitRetryRerunsAndKeepsReason(t *testing.T) {
	store := cmdtest.IsolateTicketStore(t)
	ev := tickets.LogEvent{Type: tickets.EventPauseQuestion, By: "gx", PauseKind: "spinning", Summary: "stuck"}
	body := "Body.\n\n## Needs Answer\n\nWhich way?\n"
	project, ticket := importOldTicket(t, store, "needs-repair", body, &ev)

	// A directory where the ticket's log should be makes the question step fail
	// after the frontmatter migration has run.
	logPath := tickets.LogPath(ticket)
	saved, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(logPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := serverStart(t, project); err == nil {
		t.Fatal("want the pause unit to fail")
	}
	if slices.Contains(readAppliedOrNone(project), "pause-and-question-sections") {
		t.Error("a failed pause unit was recorded as applied")
	}
	if got := readPaused(t, ticket); got.Status == schema.StatusPaused {
		t.Errorf("failed unit already paused the ticket")
	}

	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, saved, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := serverStart(t, project); err != nil {
		t.Fatalf("retry: %v", err)
	}
	got := readPaused(t, ticket)
	if got.Status != schema.StatusPaused || got.PauseReason != schema.PauseSpinning {
		t.Errorf("after retry: status %q reason %q, want paused %q", got.Status, got.PauseReason, schema.PauseSpinning)
	}
	if !slices.Contains(appliedNames(t, project), "pause-and-question-sections") {
		t.Error("retry did not record the unit")
	}
}

func readAppliedOrNone(project string) []string {
	raw, err := os.ReadFile(filepath.Join(project, migrations.AppliedFile))
	if err != nil {
		return nil
	}
	return strings.Fields(string(raw))
}

// The question reaches the ticket log before the heading is renamed: the
// question step looks for "## Needs Answer", so renaming first would lose it.
func TestExecute_TicketsMigrateToStore_NeedsAnswerQuestionIsLoggedBeforeHeadingBecomesPaused(t *testing.T) {
	store := cmdtest.IsolateTicketStore(t)
	body := "Body.\n\n## Needs Answer\n\nWhich way should it go?\n"
	project, ticket := importOldTicket(t, store, "needs-answer", body, nil)

	if _, err := serverStart(t, project); err != nil {
		t.Fatalf("server start: %v", err)
	}
	events, err := tickets.ReadLog(ticket)
	if err != nil {
		t.Fatal(err)
	}
	asked := slices.IndexFunc(events, func(e tickets.LogEvent) bool {
		return e.Type == tickets.EventPauseQuestion && strings.Contains(e.Text, "Which way should it go?")
	})
	if asked < 0 {
		t.Fatalf("question missing from the log: %+v", events)
	}
	if got := readPaused(t, ticket).Status; got != schema.StatusPaused {
		t.Errorf("status %q, want paused", got)
	}
}
