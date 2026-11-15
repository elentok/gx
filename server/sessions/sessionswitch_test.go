package sessions_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/elentok/gx/modelrules"
	"github.com/elentok/gx/server/sessions"
)

var switchList = []modelrules.Entry{{Value: "sonnet", ID: "claude-sonnet-5-5"}, {Value: "claude-sonnet-5-5", ID: "claude-sonnet-5-5"}, {Value: "opus", ID: "claude-opus-5-5"}}

func TestRecordSwitch(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		meta      sessions.Meta
		model     string
		tierModel string
		wantHand  bool
	}{
		"a different model sets the flag":      {sessions.Meta{Tier: "worker", Model: "sonnet", Effort: "low"}, "opus", "sonnet", true},
		"the tier's own model clears the flag": {sessions.Meta{Tier: "worker", Model: "opus", SwitchedByHand: true}, "sonnet", "sonnet", false},
		"an alias equals its resolved id":      {sessions.Meta{Tier: "worker", Model: "opus", SwitchedByHand: true}, "claude-sonnet-5-5", "sonnet", false},
		"no tier never clears the flag":        {sessions.Meta{Model: "opus"}, "sonnet", "sonnet", true},
		"no configured model keeps the flag":   {sessions.Meta{Tier: "worker", Model: "opus"}, "sonnet", "", true},
	} {
		meta := tc.meta
		sessions.RecordSwitch(&meta, tc.model, "high", tc.tierModel, switchList, now)
		if meta.Model != tc.model || meta.Effort != "high" || meta.Tier != tc.meta.Tier {
			t.Errorf("%s: model/effort/tier = %q/%q/%q", name, meta.Model, meta.Effort, meta.Tier)
		}
		if meta.SwitchedByHand != tc.wantHand || !meta.SwitchUnconfirmed || !meta.SwitchedAt.Equal(now) {
			t.Errorf("%s: flag/unconfirmed/at = %v/%v/%v, want %v/true/%v", name, meta.SwitchedByHand, meta.SwitchUnconfirmed, meta.SwitchedAt, tc.wantHand, now)
		}
	}
}

func TestRecordSwitch_RollbackTarget(t *testing.T) {
	t.Parallel()
	meta := sessions.Meta{Tier: "worker", Model: "sonnet", Effort: "low"}
	sessions.RecordSwitch(&meta, "opus", "high", "sonnet", switchList, time.Now())
	want := sessions.SwitchTarget{Model: "sonnet", Effort: "low"}
	if meta.SwitchPrev == nil || *meta.SwitchPrev != want {
		t.Fatalf("target = %+v, want %+v", meta.SwitchPrev, want)
	}
	// A second switch before the first message keeps the first target.
	sessions.RecordSwitch(&meta, "sonnet", "max", "sonnet", switchList, time.Now())
	if meta.SwitchPrev == nil || *meta.SwitchPrev != want {
		t.Errorf("target after a second switch = %+v, want %+v", meta.SwitchPrev, want)
	}
	// Once confirmed, the next switch records the record as it stands.
	meta.SwitchUnconfirmed = false
	sessions.RecordSwitch(&meta, "opus", "low", "sonnet", switchList, time.Now())
	if want := (sessions.SwitchTarget{Model: "sonnet", Effort: "max"}); *meta.SwitchPrev != want {
		t.Errorf("target after a confirmed switch = %+v, want %+v", meta.SwitchPrev, want)
	}
}

func TestMeta_SwitchFieldsOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(sessions.Meta{ID: "s", Model: "sonnet"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"switched_by_hand", "switch_prev", "switch_unconfirmed", "switched_at"} {
		if _, ok := got[k]; ok {
			t.Errorf("empty record writes %q: %s", k, data)
		}
	}
	var old sessions.Meta
	if err := json.Unmarshal([]byte(`{"id":"s","model":"sonnet"}`), &old); err != nil || old.SwitchedByHand || old.SwitchUnconfirmed || old.SwitchPrev != nil {
		t.Errorf("old record = %+v, %v, want no switch pending", old, err)
	}
}
