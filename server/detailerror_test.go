package server_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
)

// writeDetailEpic writes the epic and gives the project a repo, which the
// ticket lookup needs.
func writeDetailEpic(t *testing.T, store string, issues map[string]string) {
	t.Helper()
	writeEpicFiles(t, store, "work", "---\nstatus: open\n---\n", issues)
	pf := `{"name":"proj","repo":` + strconv.Quote(t.TempDir()) + `}`
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(pf), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetailWeb_BrokenFileGivesSmallPayload(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeDetailEpic(t, store, map[string]string{
			"01-a.md": "---\nid: [unclosed\n---\n\n# A\n",
		})
	})
	code, body := webDo(t, h, http.MethodGet, base+"/v1/tickets/detail?address=proj:work/01")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var d server.TicketDetail
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	if d.Status != tickets.StatusError.Word() || d.Title == "" || d.Epic != "proj:work" {
		t.Errorf("header = %+v", d)
	}
	if d.ErrorReason != "ticket file format is invalid" || d.ParseError == "" || strings.Contains(d.ParseError, "format is invalid") || !strings.HasSuffix(d.Path, filepath.Join("work", "issues", "01-a.md")) {
		t.Errorf("error fields = reason %q parse %q path %q", d.ErrorReason, d.ParseError, d.Path)
	}
	if d.Body != "" || d.Stats != nil {
		t.Errorf("body %q stats %+v, want none", d.Body, d.Stats)
	}
}

func TestDetailWeb_GraphErrorKeepsFullPayload(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeDetailEpic(t, store, map[string]string{
			"01-a.md": issueMD("01", "open", "blocked_by: [\"nope\"]\n", "A"),
		})
	})
	code, body := webDo(t, h, http.MethodGet, base+"/v1/tickets/detail?address=proj:work/01")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var d server.TicketDetail
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	if d.Status != tickets.StatusError.Word() || d.ErrorReason == "" || d.Path == "" {
		t.Errorf("error fields = %+v", d)
	}
	if d.ParseError != "" || d.Title != "A" || d.Body == "" || d.Stats == nil {
		t.Errorf("payload = title %q body %q stats %+v parse %q, want the full payload", d.Title, d.Body, d.Stats, d.ParseError)
	}
}

func TestDetailWeb_UnknownAndMalformedAddressAre404(t *testing.T) {
	h, base := startEpicDetail(t, func(store string) {
		writeDetailEpic(t, store, map[string]string{
			"01-a.md": issueMD("01", "open", "", "A"),
		})
	})
	for _, addr := range []string{"proj:work/99", "not%20an%20address", "proj:work/%%"} {
		if code, body := webDo(t, h, http.MethodGet, base+"/v1/tickets/detail?address="+addr); code != http.StatusNotFound {
			t.Errorf("%q: status %d (%s), want 404", addr, code, body)
		}
	}
}
