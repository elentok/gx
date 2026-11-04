package health

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/syshealth"
)

// fakeSessions answers Meta from a map keyed by the session directory's name.
type fakeSessions map[string]SessionMeta

func (f fakeSessions) Meta(dir string) (SessionMeta, error) {
	meta, ok := f[filepath.Base(dir)]
	if !ok {
		return SessionMeta{}, errors.New("no meta")
	}
	return meta, nil
}

func writeSession(t *testing.T, root, id string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, id), 0o755); err != nil {
		t.Fatal(err)
	}
}

// Seam 1: a session without a ticket shows its name, one with a ticket shows
// the ticket, and a holder that sends no start times gives no owner. Pings are
// cached per session run (stories 37 to 39).
func TestSessionOwners(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, id := range []string{"a", "b", "old"} {
		writeSession(t, root, id)
	}
	sessions := fakeSessions{
		"a":   {Name: "scratch"},
		"b":   {Name: "other", Ticket: "gx:foo/03"},
		"old": {Name: "old"},
	}
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
	s := &Health{cfg: Config{SessionsDir: root}, sessions: sessions}

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
