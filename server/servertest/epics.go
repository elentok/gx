package servertest

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
)

// WriteEpicFiles writes <store>/proj/<epic>/ticket.md and one issue file per
// entry of issues (file name to whole file content).
func WriteEpicFiles(t *testing.T, store, epic, ticketMD string, issues map[string]string) {
	t.Helper()
	dir := filepath.Join(store, "proj", epic)
	if err := os.MkdirAll(filepath.Join(dir, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(`{"name":"proj"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ticket.md"), []byte(ticketMD), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range issues {
		if err := os.WriteFile(filepath.Join(dir, "issues", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// WriteProjectEpic is WriteEpicFiles for any project, not only "proj".
func WriteProjectEpic(t *testing.T, store, project, epic, ticketMD string, issues map[string]string) {
	t.Helper()
	dir := filepath.Join(store, project, epic)
	if err := os.MkdirAll(filepath.Join(dir, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(store, project, "project.json"): `{"name":"` + project + `"}`,
		filepath.Join(dir, "ticket.md"):               ticketMD,
	}
	for name, content := range issues {
		files[filepath.Join(dir, "issues", name)] = content
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// IssueMD returns the whole file of an implement ticket.
func IssueMD(id, status, extra, title string) string {
	return "---\nid: \"" + id + "\"\nstatus: " + status + "\ntype: implement\n" + extra + "---\n\n# " + title + "\n"
}

// StartEpicDetail runs setup on a fresh store, starts a harness with a web
// listener on it, and returns the harness with its web URL.
func StartEpicDetail(t *testing.T, setup func(store string)) (*Harness, string) {
	t.Helper()
	store := t.TempDir()
	setup(store)
	h := StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return h, hs.WebURL
}

// GetEpicList decodes an /v1/epics response and fails on a non-200 status.
func GetEpicList(t *testing.T, code int, body string) wire.EpicList {
	t.Helper()
	var l wire.EpicList
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if err := json.Unmarshal([]byte(body), &l); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return l
}
