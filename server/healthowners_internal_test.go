package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/syshealth"
)

func writeSession(t *testing.T, root, id string, meta sessionMeta) {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionMetaFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Seam 1: a session without a ticket shows its name, one with a ticket shows
// the ticket, and a holder that sends no start times gives no owner. Pings are
// cached per session run (stories 37 to 39).
func TestSessionOwners(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "a", sessionMeta{ID: "a", Name: "scratch"})
	writeSession(t, root, "b", sessionMeta{ID: "b", Name: "other", Ticket: "gx:foo/03"})
	writeSession(t, root, "old", sessionMeta{ID: "old", Name: "old"})
	pings := map[string]int{}
	ping := func(dir string) ([]syshealth.OwnerKey, error) {
		id := filepath.Base(dir)
		pings[id]++
		switch id {
		case "a":
			return []syshealth.OwnerKey{{PID: 10, StartUnix: 5}, {PID: 11, StartUnix: 6}}, nil
		case "b":
			return []syshealth.OwnerKey{{PID: 20, StartUnix: 7}}, nil
		}
		return nil, errors.New("holder sends no start times")
	}
	s := &Server{cfg: Config{SessionsDir: root}}

	for range 2 {
		owners := map[syshealth.OwnerKey]syshealth.Owner{}
		s.addSessionOwners(owners, ping)
		want := map[syshealth.OwnerKey]syshealth.Owner{
			{PID: 10, StartUnix: 5}: {Kind: syshealth.OwnerSession, Name: "scratch"},
			{PID: 11, StartUnix: 6}: {Kind: syshealth.OwnerSession, Name: "scratch"},
			{PID: 20, StartUnix: 7}: {Kind: syshealth.OwnerTicket, Name: "gx:foo/03"},
		}
		if len(owners) != len(want) {
			t.Fatalf("owners = %v, want %v", owners, want)
		}
		for k, o := range want {
			if owners[k] != o {
				t.Errorf("owners[%v] = %v, want %v", k, owners[k], o)
			}
		}
	}
	if pings["a"] != 1 || pings["b"] != 1 {
		t.Errorf("pings = %v, want one per live session run", pings)
	}
}
