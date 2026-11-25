package chat

import (
	"net/http"
	"testing"

	"github.com/elentok/gx/server/wire/subagent"
)

func TestRead_SubagentOfAGoneParentIsNotAvailable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		fix          func(t *testing.T) *readFixture
		reason       string
		wantArchived bool
	}{
		{"claude transcript missing", newClaudeFixture, subagent.SubagentUnavailableGone, false},
		{"codex with no rollout", newCodexFixture, subagent.SubagentUnavailableGone, false},
		{"codex with only an archive", func(t *testing.T) *readFixture {
			f := newCodexFixture(t)
			writeZstd(t, f.path, archivedRollout...)
			return f
		}, subagent.SubagentUnavailableCompressed, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := tc.fix(t)
			code, body := f.getQuery(t, "/v1/sessions/chat", "subagent=x")
			got, err := decodeChat(body)
			if err != nil || code != http.StatusOK || got.Subagent == nil {
				t.Fatalf("= %d %q (%v)", code, body, err)
			}
			if got.Archived != tc.wantArchived {
				t.Errorf("archived = %v, want %v", got.Archived, tc.wantArchived)
			}
			h := got.Subagent
			if h.Status != subagent.SubagentStatusNotAvailable || h.Unavailable != tc.reason || h.CallID != "x" || h.RootSessionID != "s1" {
				t.Fatalf("header = %+v", h)
			}
			if h.Title != "" || h.Prompt != "" || h.Type != "" {
				t.Errorf("header has values of a call nobody can read: %+v", h)
			}
		})
	}
}
