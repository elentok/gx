package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// moveFixture is a project with a source epic (tickets 01 open+never run, 02
// done, 03 with a parent, 04 with run history), a target epic holding 01..02
// and a map epic.
type moveFixture struct {
	repo, project string
}

func setupMoveProject(t *testing.T) moveFixture {
	t.Helper()
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repo)
	for _, e := range []string{"src", "dst"} {
		testutil.EnsureEpicTicketMD(t, filepath.Join(project, e))
		if err := os.MkdirAll(filepath.Join(project, e, "issues"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	mapDir := filepath.Join(project, "atlas")
	if err := os.MkdirAll(mapDir, 0755); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, mapDir, "ticket.md", "---\nstatus: open\nkind: map\n---\n\n# Atlas\n")
	if err := os.MkdirAll(filepath.Join(mapDir, "issues"), 0755); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(project, "src", "issues")
	dst := filepath.Join(project, "dst", "issues")
	testutil.WriteFile(t, src, "01-first.md", "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n\n# 01 — First\n\nbody\n")
	testutil.WriteFile(t, src, "02-second.md", "---\nid: \"02\"\nstatus: open\ntype: implement\nblocked_by:\n  - \"01\"\n---\n\n# 02 — Second\n")
	testutil.WriteFile(t, src, "03-third.md", "---\nid: \"03\"\nstatus: open\ntype: implement\nblocked_by:\n  - \"01\"\n  - dst/01\nbase: \"02\"\n---\n\n# 03 — Third\n")
	testutil.WriteFile(t, src, "04-run.md", "---\nid: \"04\"\nstatus: open\ntype: implement\nsession_ids:\n  - abc\n---\n\n# 04 — Run\n")
	testutil.WriteFile(t, src, "05-done.md", "---\nid: \"05\"\nstatus: done\ntype: implement\n---\n\n# 05 — Done\n")
	testutil.WriteFile(t, src, "06a-parent.md", "---\nid: \"06a\"\nstatus: open\ntype: implement\nparent: \"05\"\n---\n\n# 06a\n")
	testutil.WriteFile(t, src, "07-fork.md", "---\nid: \"07\"\nstatus: open\ntype: implement\n---\n\n# 07\n")
	testutil.WriteFile(t, src, "07a-child.md", "---\nid: \"07a\"\nstatus: open\ntype: implement\nparent: \"07\"\n---\n\n# 07a\n")
	testutil.WriteFile(t, dst, "01-one.md", "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n\n# 01\n")
	testutil.WriteFile(t, dst, "02-two.md", "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n\n# 02\n")
	return moveFixture{repo: repo, project: project}
}

func TestExecute_TicketsMove_RenumbersAndRewritesOwnRefs(t *testing.T) {
	f := setupMoveProject(t)

	// 03 blocks on bare 01 (source epic) and dst/01 (target epic).
	out, err := runIn(t, f.repo, "tickets", "move", "src/03", "dst")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if out != "mine:dst/03\n" {
		t.Errorf("stdout = %q, want the new address", out)
	}
	if _, err := os.Stat(filepath.Join(f.project, "src", "issues", "03-third.md")); !os.IsNotExist(err) {
		t.Errorf("old file still there: %v", err)
	}
	newPath := filepath.Join(f.project, "dst", "issues", "03-third.md")
	got, err := schema.ParseTicket(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "03" || got.Status != schema.StatusOpen {
		t.Errorf("id/status = %q/%q", got.ID, got.Status)
	}
	if want := []schema.TicketID{"src/01", "01"}; !equalIDs(got.BlockedBy, want) {
		t.Errorf("blocked_by = %v, want %v", got.BlockedBy, want)
	}
	if got.Base != "src/02" {
		t.Errorf("base = %q, want src/02", got.Base)
	}
	raw, _ := os.ReadFile(newPath)
	if !strings.Contains(string(raw), "# 03 — Third") {
		t.Errorf("title prefix not rewritten:\n%s", raw)
	}
}

func TestExecute_TicketsMove_NewIDAndTitlePrefixFollowTarget(t *testing.T) {
	f := setupMoveProject(t)

	out, err := runIn(t, f.repo, "tickets", "move", "src/01", "dst", "--slug", "renamed")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if out != "mine:dst/03\n" {
		t.Errorf("stdout = %q", out)
	}
	newPath := filepath.Join(f.project, "dst", "issues", "03-renamed.md")
	raw, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `id: "03"`) || !strings.Contains(string(raw), "# 03 — First") {
		t.Errorf("id/title not rewritten:\n%s", raw)
	}
}

func TestExecute_TicketsMove_Refusals(t *testing.T) {
	f := setupMoveProject(t)
	other := addProject(t, filepath.Dir(f.project), "elsewhere", filepath.Join(t.TempDir(), "x"))
	testutil.EnsureEpicTicketMD(t, filepath.Join(other, "far"))

	for _, tc := range []struct {
		name, addr, target, code string
	}{
		{"status", "src/05", "dst", tickets.CodeMoveStatus},
		{"run history", "src/04", "dst", tickets.CodeMoveRunHistory},
		{"has parent", "src/06a", "dst", tickets.CodeMoveLineage},
		{"has fork children", "src/07", "dst", tickets.CodeMoveLineage},
		{"same epic", "src/01", "src", tickets.CodeMoveSameEpic},
		{"other project", "src/01", "elsewhere:far", tickets.CodeMoveCrossProject},
		{"missing target", "src/01", "nope", tickets.CodeUnknownEpic},
		{"unknown ticket", "src/99", "dst", tickets.CodeUnknownTicket},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := moveTreeSnapshot(t, f.project)
			_, err := runIn(t, f.repo, "tickets", "move", tc.addr, tc.target)
			var ae *tickets.AddressError
			if !errors.As(err, &ae) || ae.Code != tc.code {
				t.Fatalf("err = %v, want code %s", err, tc.code)
			}
			if after := moveTreeSnapshot(t, f.project); after != before {
				t.Errorf("refusal wrote something:\nbefore %s\nafter  %s", before, after)
			}
		})
	}
}

func TestExecute_TicketsMove_MapEpicSourceRefusedTargetAllowed(t *testing.T) {
	f := setupMoveProject(t)
	testutil.WriteFile(t, filepath.Join(f.project, "atlas", "issues"), "01-m.md",
		"---\nid: \"01\"\nstatus: open\ntype: implement\n---\n\n# 01\n")

	_, err := runIn(t, f.repo, "tickets", "move", "atlas/01", "dst")
	var ae *tickets.AddressError
	if !errors.As(err, &ae) || ae.Code != tickets.CodeMoveMapSource {
		t.Fatalf("map source err = %v, want %s", err, tickets.CodeMoveMapSource)
	}

	out, err := runIn(t, f.repo, "tickets", "move", "src/01", "atlas")
	if err != nil {
		t.Fatalf("move into map: %v", err)
	}
	if out != "mine:atlas/02\n" {
		t.Errorf("stdout = %q", out)
	}
}

func TestExecute_TicketsMove_DryRunWritesNothing(t *testing.T) {
	f := setupMoveProject(t)
	before := moveTreeSnapshot(t, f.project)

	out, err := runIn(t, f.repo, "tickets", "move", "src/03", "dst", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if !strings.Contains(out, "mine:src/03") || !strings.Contains(out, "mine:dst/03") {
		t.Errorf("plan = %q, want from and to", out)
	}
	if after := moveTreeSnapshot(t, f.project); after != before {
		t.Errorf("dry-run wrote something")
	}
}

func TestExecute_TicketsMove_JSONShape(t *testing.T) {
	f := setupMoveProject(t)

	out, err := runIn(t, f.repo, "tickets", "move", "src/03", "dst", "--json")
	if err != nil {
		t.Fatalf("move --json: %v", err)
	}
	m := decodeJSON(t, []byte(out))
	if m["from"] != "mine:src/03" || m["to"] != "mine:dst/03" || m["via"] != "direct" {
		t.Errorf("json = %v", m)
	}
	if p, _ := m["path"].(string); p != filepath.Join(f.project, "dst", "issues", "03-third.md") {
		t.Errorf("path = %v", m["path"])
	}
	if _, ok := m["rewritten"].([]any); !ok {
		t.Errorf("rewritten = %v, want an array", m["rewritten"])
	}
}

func TestRewriteMovedRef(t *testing.T) {
	for _, tc := range []struct{ ref, want string }{
		{"01", "src/01"},
		{"04a", "src/04a"},
		{"dst/02", "02"},
		{"mine:dst/02", "02"},
		{"other/02", "other/02"},
		{"ralph-loop/branch", "ralph-loop/branch"},
		{"main", "main"},
	} {
		if got := rewriteMovedRef(tc.ref, "src", "dst", "mine"); got != tc.want {
			t.Errorf("rewriteMovedRef(%q) = %q, want %q", tc.ref, got, tc.want)
		}
	}
}

func equalIDs(a, b []schema.TicketID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// moveTreeSnapshot is every file under root with its content, for "nothing was
// written" assertions. Lock sidecars are skipped: they come and go.
func moveTreeSnapshot(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".lock") {
			return err
		}
		data, err := os.ReadFile(p)
		b.WriteString(p + "\n" + string(data) + "\n")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
