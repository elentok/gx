package sessions

import (
	"encoding/json"
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestUpdateSessionMeta_UnreadableMetaIsAnError(t *testing.T) {
	t.Parallel()
	mg := &Manager{Watches: NewSessionWatches()}
	err := mg.UpdateSessionMeta(t.TempDir(), "abcdef", func(*Meta) bool {
		t.Error("change ran on an unreadable meta")
		return false
	})
	if err == nil {
		t.Error("update of an unreadable meta = nil, want an error")
	}
}

func TestMeta_FlowReadsAndInfo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		json       string
		flow       tickets.Flow
		design     bool
		stepped    bool
		wantKey    string
		wantReSave string
	}{
		{"old design meta", `{"id":"a","design":true,"ticket":"p:e/01"}`, tickets.FlowDesign, true, true, "p:e/01", "design"},
		{"triage", `{"id":"a","flow":"triage","flow_key":"proj"}`, tickets.FlowTriage, false, true, "proj", "triage"},
		{"plain", `{"id":"a"}`, "", false, false, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var m Meta
			if err := json.Unmarshal([]byte(tc.json), &m); err != nil {
				t.Fatal(err)
			}
			if m.Flow != tc.flow || m.IsDesign() != tc.design || m.IsStepped() != tc.stepped {
				t.Errorf("flow=%q design=%v stepped=%v, want %q %v %v", m.Flow, m.IsDesign(), m.IsStepped(), tc.flow, tc.design, tc.stepped)
			}
			info := m.Info()
			if info.Flow != string(tc.flow) || info.FlowKey != tc.wantKey || info.Design != tc.design {
				t.Errorf("info flow=%q key=%q design=%v, want %q %q %v", info.Flow, info.FlowKey, info.Design, tc.flow, tc.wantKey, tc.design)
			}
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if err := json.Unmarshal(b, &raw); err != nil {
				t.Fatal(err)
			}
			if _, has := raw["design"]; has {
				t.Error("saved meta still carries the legacy design bool")
			}
			if got, _ := raw["flow"].(string); got != tc.wantReSave {
				t.Errorf("saved flow = %q, want %q", got, tc.wantReSave)
			}
		})
	}
}
