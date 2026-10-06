package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

// writeOldTree builds an old .scratch tree with a ticket, an archived ticket
// and an event log, returning its root.
func writeOldTree(t *testing.T, status string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".scratch")
	files := map[string]string{
		"widget/issues/01-first.md":          "---\nid: \"01\"\nstatus: " + status + "\ntype: task\n---\nBody.\n",
		"widget/.archive/00-old.md":          "---\nid: \"00\"\nstatus: done\ntype: task\n---\nOld.\n",
		"widget/events.jsonl":                "{\"event\":\"x\"}\n",
		".archive/gone/issues/01-ancient.md": "---\nid: \"01\"\nstatus: done\ntype: task\n---\nAncient.\n",
	}
	for rel, content := range files {
		dir := filepath.Join(root, filepath.Dir(rel))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.WriteFile(t, dir, filepath.Base(rel), content)
	}
	return root
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(p)
		out[p] = string(raw)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExecute_TicketsMigrateToStore_CopiesTreeAndCreatesProject(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	old := writeOldTree(t, "open")
	before := snapshotTree(t, old)

	out, err := runIn(t, repo, "tickets", "migrate", "--to-store", "--project", "mine", old)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(out, `copied 4 file(s) into project "mine"`) {
		t.Errorf("stdout = %q", out)
	}

	project := filepath.Join(store, "mine")
	for rel, want := range before {
		relPath, _ := filepath.Rel(old, rel)
		got, err := os.ReadFile(filepath.Join(project, relPath))
		if err != nil {
			t.Fatalf("copied file %s: %v", relPath, err)
		}
		if strings.Contains(want, "type: task") {
			// Tickets are converted on the way in; the shape is checked in the
			// conversion test.
			if !strings.Contains(string(got), "type: implement") {
				t.Errorf("%s not converted: %q", relPath, got)
			}
		} else if string(got) != want {
			t.Errorf("%s = %q, want %q", relPath, got, want)
		}
	}
	pf, err := os.ReadFile(filepath.Join(project, "project.json"))
	if err != nil {
		t.Fatalf("project.json: %v", err)
	}
	if !strings.Contains(string(pf), `"name": "mine"`) || !strings.Contains(string(pf), filepath.Base(repo)) {
		t.Errorf("project.json = %s", pf)
	}

	// The project is now what `tickets root` resolves for the repo.
	root, err := runIn(t, repo, "tickets", "root")
	if err != nil || strings.TrimSpace(root) != project {
		t.Errorf("tickets root = %q, %v; want %q", root, err, project)
	}

	if after := snapshotTree(t, old); len(after) != len(before) {
		t.Errorf("old tree changed: %v", after)
	} else {
		for p, v := range before {
			if after[p] != v {
				t.Errorf("old tree file %s changed", p)
			}
		}
	}
}

func TestExecute_TicketsMigrateToStore_ConvertsEpicShapeAndTypes(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	root := filepath.Join(t.TempDir(), ".scratch")
	files := map[string]string{
		"sidecar/epic.yaml":          "started_at: 2026-01-02T03:04:05Z\ncompleted_at: 2026-01-03T03:04:05Z\n",
		"sidecar/issues/01-first.md": "---\nid: \"01\"\nstatus: ready-for-agent\ntype: task\n---\nBody.\n",
		"sidecar/.archive/00-old.md": "---\nid: \"00\"\nstatus: done\ntype: task\n---\nOld.\n",
		"mapped/map.md":              "# The map\n\nPlans.\n",
		"mapped/issues/01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\nBody.\n",
		"already/ticket.md":          "---\nstatus: open\n---\nKept.\n",
		"already/issues/01-first.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\nBody.\n",
	}
	for rel, content := range files {
		dir := filepath.Join(root, filepath.Dir(rel))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.WriteFile(t, dir, filepath.Base(rel), content)
	}

	if _, err := runIn(t, repo, "tickets", "migrate", "--to-store", "--project", "mine", root); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	project := filepath.Join(store, "mine")
	read := func(rel string) string {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(project, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		return string(got)
	}

	for _, gone := range []string{"sidecar/epic.yaml", "mapped/map.md"} {
		if _, err := os.Stat(filepath.Join(project, gone)); !os.IsNotExist(err) {
			t.Errorf("%s should not be copied: %v", gone, err)
		}
	}
	sidecar := read("sidecar/ticket.md")
	for _, want := range []string{"status: done", "started_at: 2026-01-02T03:04:05Z", "completed_at: 2026-01-03T03:04:05Z"} {
		if !strings.Contains(sidecar, want) {
			t.Errorf("sidecar ticket.md missing %q:\n%s", want, sidecar)
		}
	}
	mapped := read("mapped/ticket.md")
	if !strings.Contains(mapped, "status: open") || !strings.HasSuffix(mapped, "# The map\n\nPlans.\n") {
		t.Errorf("mapped ticket.md = %q", mapped)
	}
	if got := read("already/ticket.md"); got != files["already/ticket.md"] {
		t.Errorf("existing ticket.md rewritten: %q", got)
	}

	for _, rel := range []string{"sidecar/issues/01-first.md", "sidecar/.archive/00-old.md", "mapped/issues/01-first.md"} {
		got := read(rel)
		if !strings.Contains(got, "type: implement") || strings.Contains(got, "type: task") {
			t.Errorf("%s type not converted:\n%s", rel, got)
		}
	}
	if got := read("sidecar/issues/01-first.md"); !strings.Contains(got, "status: open") || strings.Contains(got, "ready-for-agent") {
		t.Errorf("legacy status not fixed:\n%s", got)
	}
	if got := read("already/issues/01-first.md"); got != files["already/issues/01-first.md"] {
		t.Errorf("already-converted ticket rewritten: %q", got)
	}
}

func TestExecute_TicketsMigrateToStore_NameDefaultsToRepoDir(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	old := writeOldTree(t, "open")

	if _, err := runIn(t, repo, "tickets", "migrate", "--to-store", old); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store, filepath.Base(repo), "project.json")); err != nil {
		t.Errorf("want project named after the repo dir: %v", err)
	}
}

func TestExecute_TicketsMigrateToStore_RerunRefusesPerExistingAddress(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	old := writeOldTree(t, "open")
	if _, err := runIn(t, repo, "tickets", "migrate", "--to-store", "--project", "mine", old); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	ticket := filepath.Join(store, "mine", "widget", "issues", "01-first.md")
	if err := os.WriteFile(ticket, []byte("edited in store\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runIn(t, repo, "tickets", "migrate", "--to-store", "--project", "mine", old)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("err = %v, want overwrite refusal", err)
	}
	if got, _ := os.ReadFile(ticket); string(got) != "edited in store\n" {
		t.Errorf("store ticket overwritten: %q", got)
	}
}

func TestExecute_TicketsMigrateToStore_RefusesWhileTicketClaimed(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	old := writeOldTree(t, "claimed")

	_, err := runIn(t, repo, "tickets", "migrate", "--to-store", "--project", "mine", old)
	if err == nil || !strings.Contains(err.Error(), "claimed") {
		t.Fatalf("err = %v, want claimed refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(store, "mine")); !os.IsNotExist(statErr) {
		t.Errorf("store written despite refusal: %v", statErr)
	}
}

func writeInvalidTicket(t *testing.T, old string) {
	t.Helper()
	dir := filepath.Join(old, "widget", "issues")
	testutil.WriteFile(t, dir, "02-broken.md", "---\nid: \"02\"\nstatus: bogus\ntype: task\n---\nBroken.\n")
}

func TestExecute_TicketsMigrateToStore_InvalidTicketRefusesNamingAddress(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	old := writeOldTree(t, "open")
	writeInvalidTicket(t, old)

	_, err := runIn(t, repo, "tickets", "migrate", "--to-store", "--project", "mine", old)
	want := filepath.Join(store, "mine", "widget", "issues", "02-broken.md")
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want it to name %s", err, want)
	}
	if _, statErr := os.Stat(filepath.Join(store, "mine")); !os.IsNotExist(statErr) {
		t.Errorf("store written despite invalid ticket: %v", statErr)
	}
}

func TestExecute_TicketsMigrateToStore_DryRunValidatesWithoutWriting(t *testing.T) {
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	old := writeOldTree(t, "open")

	out, err := runIn(t, repo, "tickets", "migrate", "--to-store", "--dry-run", "--project", "mine", old)
	if err != nil || !strings.Contains(out, "dry run") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if _, statErr := os.Stat(filepath.Join(store, "mine")); !os.IsNotExist(statErr) {
		t.Errorf("dry run wrote to the store: %v", statErr)
	}

	writeInvalidTicket(t, old)
	_, err = runIn(t, repo, "tickets", "migrate", "--to-store", "--dry-run", "--project", "mine", old)
	if err == nil || !strings.Contains(err.Error(), "02-broken.md") {
		t.Fatalf("err = %v, want it to name 02-broken.md", err)
	}
}
