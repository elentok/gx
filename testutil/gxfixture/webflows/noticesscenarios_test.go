package webflows

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestAgentsNotices_EachSessionCarriesOneKind(t *testing.T) {
	t.Parallel()
	r := start(t, "agents-notices")
	defer r.Stop(t)
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("sessions = %d, want 2", len(list))
	}
	for _, kind := range []string{wire.SessionNoticeUntrustedRead, wire.SessionNoticeNoComposerMarker} {
		n := 0
		for _, si := range list {
			if slices.Contains(si.Notices, kind) {
				n++
				if len(si.Notices) != 1 {
					t.Errorf("session %s notices = %v, want only %q", si.ID, si.Notices, kind)
				}
			}
		}
		if n != 1 {
			t.Errorf("sessions with notice %q = %d, want 1", kind, n)
		}
	}
}
