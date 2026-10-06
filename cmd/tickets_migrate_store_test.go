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
		if string(got) != want {
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
