package tickets

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if issuesDir := filepath.Dir(path); filepath.Base(issuesDir) == "issues" {
		testutil.EnsureEpicTicketMD(t, filepath.Dir(issuesDir))
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_MissingScratchDirReturnsEmpty(t *testing.T) {
	epics, err := Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 0 {
		t.Fatalf("expected no epics, got %v", epics)
	}
}

func TestLoad_EmptyScratchDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 0 {
		t.Fatalf("expected no epics, got %v", epics)
	}
}

func TestLoad_DiscoversEpicsAndTickets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "01-first-ticket.md"),
		"---\nid: \"01\"\nstatus: done\ntype: implement\n---\nBody.\n")
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "02-second-ticket.md"),
		"---\nid: \"02\"\nstatus: open\ntype: implement\n---\nBody.\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(epics))
	}

	epic := epics[0]
	if epic.Name != "my-epic" {
		t.Errorf("Name = %q, want %q", epic.Name, "my-epic")
	}
	if epic.IsMap {
		t.Error("epic without ticket.md should not be IsMap")
	}
	if epic.TotalCount() != 2 {
		t.Fatalf("expected 2 tickets, got %d", epic.TotalCount())
	}
	if epic.OpenCount() != 1 {
		t.Errorf("OpenCount = %d, want 1 (one done, one open)", epic.OpenCount())
	}

	byNumber := map[int]Ticket{}
	for _, tk := range epic.Tickets {
		byNumber[tk.Number] = tk
	}
	if byNumber[1].Title != "First ticket" {
		t.Errorf("ticket 1 Title = %q, want %q", byNumber[1].Title, "First ticket")
	}
	if byNumber[2].Title != "Second ticket" {
		t.Errorf("ticket 2 Title = %q, want %q", byNumber[2].Title, "Second ticket")
	}
}

func TestLoad_MirrorsMutes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "01-ticket.md"),
		"---\nid: \"01\"\nstatus: open\ntype: implement\nmutes:\n  - event_type: iteration-stalled\n    tripped_at: 2026-08-13T10:00:00Z\n---\nBody.\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := epics[0].Tickets[0].Mutes
	want := []schema.MuteRecord{
		{EventType: "iteration-stalled", TrippedAt: time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)},
	}
	if len(got) != len(want) || got[0].EventType != want[0].EventType || !got[0].TrippedAt.Equal(want[0].TrippedAt) {
		t.Errorf("Mutes = %+v, want %+v", got, want)
	}
}

func TestLoad_StrayMapMdIsNotAMapEpic(t *testing.T) {
	dir := t.TempDir()
	epicPath := filepath.Join(dir, "old-shape-epic")
	writeFile(t, filepath.Join(epicPath, "map.md"), "# Map\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 1 || epics[0].IsMap || epics[0].HasTicketMD {
		t.Fatalf("expected 1 non-map epic without ticket.md, got %+v", epics)
	}
	if IsMapEpic(epicPath) {
		t.Error("IsMapEpic should ignore a stray map.md")
	}
}

func TestLoad_ExcludesDotPrefixedDirectories(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "real-epic", "issues", "01-first-ticket.md"),
		"---\nid: \"01\"\nstatus: open\ntype: implement\n---\nBody.\n")
	writeFile(t, filepath.Join(dir, ".archive", "old-epic", "issues", "01-old-ticket.md"),
		"---\nid: \"01\"\nstatus: done\ntype: implement\n---\nBody.\n")
	writeFile(t, filepath.Join(dir, ".scratch-tmp", "issues", "01-tmp-ticket.md"),
		"---\nid: \"01\"\nstatus: open\ntype: implement\n---\nBody.\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 1 || epics[0].Name != "real-epic" {
		t.Fatalf("expected only real-epic, got %+v", epics)
	}
}

func TestLoad_EpicWithNoIssuesDirHasZeroTickets(t *testing.T) {
	dir := t.TempDir()
	testutil.EnsureEpicTicketMD(t, filepath.Join(dir, "bare-epic"))

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 1 || epics[0].TotalCount() != 0 {
		t.Fatalf("expected 1 zero-ticket epic, got %+v", epics)
	}
}

func TestLoad_UnreadableTicketFileShowsErrorRow(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: unreadable-file permissions aren't enforced")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "epic", "issues", "01-broken.md")
	writeFile(t, path, "Status: open\n\nBody.\n")
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 1 || epics[0].TotalCount() != 1 {
		t.Fatalf("expected 1 epic with 1 ticket, got %+v", epics)
	}

	tk := epics[0].Tickets[0]
	if tk.Number != 1 {
		t.Errorf("Number = %d, want 1", tk.Number)
	}
	if tk.Title != "Broken" {
		t.Errorf("Title = %q, want %q", tk.Title, "Broken")
	}
	if tk.ReadErr == "" {
		t.Error("expected ReadErr to be set for an unreadable ticket file")
	}
}

func TestLoad_IgnoresNonTicketFilesInIssuesDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "epic", "issues", "01-a-ticket.md"),
		"---\nid: \"01\"\nstatus: open\ntype: implement\n---\n")
	writeFile(t, filepath.Join(dir, "epic", "issues", "README.md"), "not a ticket\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if epics[0].TotalCount() != 1 {
		t.Fatalf("expected 1 ticket (README.md ignored), got %d", epics[0].TotalCount())
	}
}

func TestLoad_SurfacesParent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "epic", "issues", "03-original.md"),
		"---\nid: \"03\"\nstatus: done\ntype: implement\n---\n")
	writeFile(t, filepath.Join(dir, "epic", "issues", "03a-first-half.md"),
		"---\nid: \"03a\"\nstatus: open\ntype: implement\nparent: \"03\"\n---\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(epics) != 1 || len(epics[0].Tickets) != 2 {
		t.Fatalf("expected 1 epic with 2 tickets, got %+v", epics)
	}

	byIdentifier := map[string]Ticket{}
	for _, tk := range epics[0].Tickets {
		byIdentifier[tk.Identifier] = tk
	}

	if original := byIdentifier["03"]; original.Parent != nil {
		t.Errorf("original.Parent = %v, want nil", original.Parent)
	}

	child := byIdentifier["03a"]
	if child.Parent == nil || *child.Parent != "03" {
		t.Errorf("child.Parent = %v, want \"03\"", child.Parent)
	}
}

// TestLoad_RejectsPreContractionShape pins that the loader surfaces a
// pre-migration ticket as a read error rather than quietly ignoring the parts
// it no longer understands — the point of the contraction is that the old
// shape can't come back through a compatibility path.
func TestLoad_RejectsPreContractionShape(t *testing.T) {
	cases := map[string]string{
		"01-children.md":    "---\nid: \"01\"\nstatus: open\ntype: implement\nchildren: [\"01a\"]\n---\n",
		"02-no-status.md":   "---\nid: \"02\"\ntype: implement\n---\n",
		"03-dead-status.md": "---\nid: \"03\"\nstatus: ready-for-human\ntype: implement\n---\n",
	}
	dir := t.TempDir()
	for name, content := range cases {
		writeFile(t, filepath.Join(dir, "epic", "issues", name), content)
	}

	epics, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(epics) != 1 || len(epics[0].Tickets) != len(cases) {
		t.Fatalf("expected 1 epic with %d tickets, got %+v", len(cases), epics)
	}
	for _, tk := range epics[0].Tickets {
		if tk.ReadErr == "" {
			t.Errorf("ticket %s loaded cleanly, want a read error", tk.Identifier)
		}
		if got := epics[0].RenderedStatus(tk); got != StatusError {
			t.Errorf("ticket %s rendered status = %v, want error", tk.Identifier, got.Word())
		}
	}
}

func TestLoad_DiscoversAlphabeticallySuffixedTicketNumbers(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"10a-first.md", "10b-second.md", "10c-third.md"} {
		id := name[:3]
		writeFile(t, filepath.Join(dir, "epic", "issues", name),
			"---\nid: \""+id+"\"\nstatus: open\ntype: implement\n---\n")
	}

	epics, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(epics) != 1 || len(epics[0].Tickets) != 3 {
		t.Fatalf("expected 3 suffixed tickets, got %+v", epics)
	}
	got := []string{epics[0].Tickets[0].DisplayNumber(), epics[0].Tickets[1].DisplayNumber(), epics[0].Tickets[2].DisplayNumber()}
	want := []string{"10a", "10b", "10c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ticket identifiers = %v, want %v", got, want)
	}
}

func TestLoad_TicketMDFrontmatterSuppliesEpicFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "new-epic", "ticket.md"),
		"---\nstatus: open\nblocked_by:\n  - other\nbase: main\nstarted_at: 2026-01-02T03:04:05Z\ncompleted_at: 2026-01-03T03:04:05Z\n---\nPlan link.\n")
	writeFile(t, filepath.Join(dir, "new-epic", "issues", "01-first.md"),
		"---\nid: \"01\"\nstatus: open\ntype: implement\n---\nBody.\n")

	epics, err := Load(dir)
	if err != nil || len(epics) != 1 {
		t.Fatalf("Load = %v, %v", epics, err)
	}
	epic := epics[0]
	if !epic.HasTicketMD || epic.Status != "open" || epic.Base != "main" ||
		!reflect.DeepEqual(epic.BlockedBy, []string{"other"}) {
		t.Errorf("ticket.md fields not read: %+v", epic)
	}
	if d, ok := epic.CompletionDuration(); !ok || d != 24*time.Hour {
		t.Errorf("CompletionDuration = %v, %v", d, ok)
	}
	if len(epic.Tickets) != 1 {
		t.Errorf("want 1 child, got %d", len(epic.Tickets))
	}
}

func TestLoad_TicketMDKindMapMakesAMapEpic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "map-epic", "ticket.md"),
		"---\nkind: map\nstatus: open\n---\n# Map\n\n## Destination\n")
	writeFile(t, filepath.Join(dir, "plain-epic", "ticket.md"), "---\nstatus: open\n---\nPlan link.\n")

	epics, err := Load(dir)
	if err != nil || len(epics) != 2 {
		t.Fatalf("Load = %v, %v", epics, err)
	}
	if m := epics[0]; !m.IsMap || m.MapBody != "# Map\n\n## Destination\n" {
		t.Errorf("kind: map epic: IsMap=%v MapBody=%q", m.IsMap, m.MapBody)
	}
	if epics[1].IsMap {
		t.Error("ticket.md without kind: map should not be IsMap")
	}
}
