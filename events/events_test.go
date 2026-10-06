package events

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidate_KindRequiredOnFailureAndRecoveryEvents(t *testing.T) {
	for _, typ := range []Type{LaunchFailed, SubmitRefused, Deadlocked, RecoveryMatched, RecoveryApplied, RecoveryProposed, RecoveryEscalated} {
		if err := Validate(typ, ""); err == nil {
			t.Errorf("Validate(%q, \"\") = nil, want missing-kind error", typ)
		}
		if err := Validate(typ, ZeroCommit); err != nil {
			t.Errorf("Validate(%q, zero-commit) = %v", typ, err)
		}
	}
}

func TestValidate_KindOptionalOnOtherEvents(t *testing.T) {
	if err := Validate(IterationStarted, ""); err != nil {
		t.Errorf("Validate(iteration-started, \"\") = %v", err)
	}
	if err := Validate(Submitted, ""); err != nil {
		t.Errorf("Validate(submitted, \"\") = %v", err)
	}
}

func TestValidate_UnknownKindRejected(t *testing.T) {
	if err := Validate(NeedsRepair, "made-up"); err == nil {
		t.Error("unknown kind accepted")
	}
}

func TestKind_CauseHerdrIsAnAttributeOfTheKind(t *testing.T) {
	if !AgentNameTaken.CauseHerdr() || !HandleMismatch.CauseHerdr() {
		t.Error("herdr-caused kinds not flagged")
	}
	if ZeroCommit.CauseHerdr() || Spinning.CauseHerdr() {
		t.Error("non-herdr kinds flagged")
	}
	for _, k := range Kinds() {
		if !k.Valid() {
			t.Errorf("Kinds() returned invalid %q", k)
		}
	}
}

func TestFit_TruncatesFieldsInOrderUnderCap(t *testing.T) {
	ev := struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
		Body   string `json:"body"`
	}{"x", strings.Repeat("é\"", 5000), "keep"}
	data, err := Fit(func() ([]byte, error) { return json.Marshal(ev) }, &ev.Reason, &ev.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(data)+1 > MaxLineBytes {
		t.Errorf("line is %d bytes", len(data)+1)
	}
	if ev.Body != "keep" {
		t.Errorf("Body touched before Reason was exhausted: %q", ev.Body)
	}
}

func TestFit_ErrorsWhenNothingLeftToCut(t *testing.T) {
	ev := struct {
		Type string `json:"type"`
	}{strings.Repeat("a", 5000)}
	if _, err := Fit(func() ([]byte, error) { return json.Marshal(ev) }); err == nil {
		t.Error("oversized line accepted")
	}
}
