package tickettest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/sessionlookup"
)

// SessionLookup answers a session lookup from sessions (id -> markdown); an
// unknown id is Missing. Assign it to rowactions.LookupSession.
func SessionLookup(sessions map[string]string) func(config.Dirs, string, string) (sessionlookup.Session, error) {
	return func(_ config.Dirs, _, id string) (sessionlookup.Session, error) {
		md, ok := sessions[id]
		if !ok {
			return sessionlookup.Session{Source: sessionlookup.Missing}, nil
		}
		return sessionlookup.Session{Source: sessionlookup.Claude, Markdown: md}, nil
	}
}

// WriteTranscriptTicket writes a done ticket whose frontmatter carries
// sessionIDs (a raw `session_ids:` block, or "") and returns its path.
func WriteTranscriptTicket(t *testing.T, sessionIDs string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-x.md")
	raw := "---\nid: 01\nstatus: done\ntype: implement\n" + sessionIDs + "---\n\n# 01 — x\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
