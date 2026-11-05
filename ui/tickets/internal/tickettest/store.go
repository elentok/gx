package tickettest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

// WriteTicket writes a legacy-format fixture (see LegacyTicketToFrontmatter)
// as a frontmatter ticket under root/.scratch/<epic>/issues.
func WriteTicket(t *testing.T, root, epic, filename, content string) {
	t.Helper()
	path := filepath.Join(root, ".scratch", epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Dir(filepath.Dir(path)))
	if err := os.WriteFile(path, []byte(LegacyTicketToFrontmatter(filename, content)), 0644); err != nil {
		t.Fatal(err)
	}
}

// WriteArchivedTicket mirrors WriteTicket but writes under `.scratch/
// .archive/<epic>/issues/<filename>` (tickets.ArchiveDir's layout), so a test
// can populate an archived epic the lazy load will pick up.
func WriteArchivedTicket(t *testing.T, root, epic, filename, content string) {
	t.Helper()
	path := filepath.Join(root, ".scratch", ".archive", epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Dir(filepath.Dir(path)))
	if err := os.WriteFile(path, []byte(LegacyTicketToFrontmatter(filename, content)), 0644); err != nil {
		t.Fatal(err)
	}
}

// WriteMap writes a map-kind epic ticket.md under root/.scratch/<epic>.
func WriteMap(t *testing.T, root, epic, content string) {
	t.Helper()
	epicDir := filepath.Join(root, ".scratch", epic)
	testutil.Mkdir(t, epicDir)
	testutil.WriteFile(t, epicDir, "ticket.md", "---\nkind: map\nstatus: open\n---\n"+content)
}

// WriteFrontmatterTicket writes a ticket file with real frontmatter fields
// (id/status/parent), unlike WriteTicket's LegacyTicketToFrontmatter
// conversion which doesn't carry parent.
func WriteFrontmatterTicket(t *testing.T, root, epic, filename, id, status, parent string) {
	t.Helper()
	path := filepath.Join(root, ".scratch", epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Dir(filepath.Dir(path)))
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %q\nstatus: %s\ntype: implement\n", id, status)
	if parent != "" {
		fmt.Fprintf(&b, "parent: %q\n", parent)
	}
	b.WriteString("---\nBody.\n")
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

// AddStoreProject registers repoRoot as a project in a temp ticket store,
// returning the project dir and the store path for the model's settings.
func AddStoreProject(t *testing.T, repoRoot string) (project, store string) {
	t.Helper()
	store = t.TempDir()
	project = filepath.Join(store, "p")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]string{"name": "p", "repo": repoRoot})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "project.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	return project, store
}

// TicketPath is the path of ticket name under root's scratch epic.
func TicketPath(root, epic, name string) string {
	return filepath.Join(root, ".scratch", epic, "issues", name)
}

// WriteRawQueueTicket writes content verbatim over an existing ticket path.
func WriteRawQueueTicket(t *testing.T, root, epic, name, content string) {
	t.Helper()
	if err := os.WriteFile(TicketPath(root, epic, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// WriteStoreTicket writes name under store's gx:alpha epic and returns its path.
func WriteStoreTicket(t *testing.T, store, name, raw string) string {
	t.Helper()
	dir := filepath.Join(store, "gx", "alpha", "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

// ReadStatus returns the status: value of the ticket file, or "".
func ReadStatus(t *testing.T, file string) string {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(line, "status: "); ok {
			return v
		}
	}
	return ""
}

// LegacyTicketToFrontmatter converts the pre-existing test fixtures (plain
// "Status: x\nBlocked by: y, z\n\nBody" lines, predating the retirement of
// old-format parsing) into a minimal valid frontmatter ticket, deriving id from
// filename so blocked_by cross-references between fixture tickets keep resolving.
func LegacyTicketToFrontmatter(filename, content string) string {
	id := filename
	if idx := strings.Index(id, "-"); idx >= 0 {
		id = id[:idx]
	}

	status, typ, pauseReason := "open", "implement", ""
	var blockedBy []string
	lines := strings.Split(content, "\n")
	bodyStart := 0
	for _, line := range lines {
		matched := true
		switch {
		case strings.HasPrefix(line, "Status: "):
			status = strings.TrimPrefix(line, "Status: ")
		case strings.HasPrefix(line, "Pause reason: "):
			pauseReason = strings.TrimPrefix(line, "Pause reason: ")
		case strings.HasPrefix(line, "Blocked by: "):
			blockedBy = strings.Split(strings.TrimPrefix(line, "Blocked by: "), ", ")
		case strings.HasPrefix(line, "Type: "):
			typ = strings.TrimPrefix(line, "Type: ")
		default:
			matched = false
		}
		if !matched {
			break
		}
		bodyStart++
	}
	body := strings.TrimPrefix(strings.Join(lines[bodyStart:], "\n"), "\n")

	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %q\nstatus: %s\ntype: %s\n", id, status, typ)
	if pauseReason != "" {
		fmt.Fprintf(&b, "pause_reason: %s\n", pauseReason)
	}
	if len(blockedBy) > 0 {
		fmt.Fprintf(&b, "blocked_by: [\"%s\"]\n", strings.Join(blockedBy, "\", \""))
	}
	b.WriteString("---\n")
	b.WriteString(body)
	return b.String()
}
