package prompt

import (
	"testing"

	"github.com/elentok/gx/server/sessions"
)

func TestSentEntry_OnlyDesignAndPlainChatSave(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		meta sessions.Meta
		want bool
	}{
		"plain chat": {sessions.Meta{}, true},
		"design":     {sessions.Meta{Flow: "design", Ticket: "proj:map"}, true},
		"triage":     {sessions.Meta{Flow: "triage", FlowKey: "proj"}, false},
		"fix agent":  {sessions.Meta{FixOf: "proj"}, false},
		"ticket":     {sessions.Meta{Ticket: "proj:epic/01"}, false},
	} {
		if _, ok := sentEntry(tc.meta, "hi"); ok != tc.want {
			t.Errorf("%s: saves = %v, want %v", name, ok, tc.want)
		}
	}
}
