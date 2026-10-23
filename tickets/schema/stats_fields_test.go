package schema

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func roundTrip(t *testing.T, in Ticket) (Ticket, string) {
	t.Helper()
	out, err := MarshalTicket(in, "\n# t\n")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseTicketFromRaw(string(out), "01-t.md")
	if err != nil {
		t.Fatal(err)
	}
	return got, string(out)
}

func TestTicket_StatsFieldsRoundTrip(t *testing.T) {
	tokens := int64(12345)
	want := Ticket{
		ID:              "01",
		Status:          StatusDone,
		Type:            TypeImplement,
		CreatedAt:       time.Date(2026, 10, 1, 1, 2, 3, 0, time.UTC),
		CancelledAt:     time.Date(2026, 10, 2, 1, 2, 3, 0, time.UTC),
		OriginalAddress: "gx:epic/01",
		TotalTokens:     &tokens,
		ActualCost:      1.5,
		HadCodex:        true,
		Friction: &FrictionSummary{
			PausesByReason:  map[string]int{"asking": 2},
			RunEvents:       map[string]int{"compaction": 1},
			PauseEndings:    map[string]int{"person": 1, "auto": 1},
			TotalPausedSecs: 90,
			PersonPauseSecs: []int{30, 45},
		},
	}
	got, _ := roundTrip(t, want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the ticket:\n got %+v\nwant %+v", got, want)
	}
}

func TestTicket_UnstampedWritesNoStatsFields(t *testing.T) {
	_, out := roundTrip(t, Ticket{ID: "01", Status: StatusOpen, Type: TypeImplement})
	for _, key := range []string{"created_at", "original_address", "cancelled_at", "total_tokens", "had_codex", "friction"} {
		if strings.Contains(out, key) {
			t.Errorf("unstamped ticket wrote %s:\n%s", key, out)
		}
	}
}

func TestTicket_ZeroTotalTokensDiffersFromUnstamped(t *testing.T) {
	zero := int64(0)
	got, out := roundTrip(t, Ticket{ID: "01", Status: StatusDone, Type: TypeImplement, TotalTokens: &zero})
	if got.TotalTokens == nil || *got.TotalTokens != 0 {
		t.Errorf("TotalTokens = %v, want pointer to 0\n%s", got.TotalTokens, out)
	}
	got, _ = roundTrip(t, Ticket{ID: "01", Status: StatusDone, Type: TypeImplement})
	if got.TotalTokens != nil {
		t.Errorf("unstamped TotalTokens = %v, want nil", *got.TotalTokens)
	}
}

func TestTicket_NegativeTotalTokensFailsValidate(t *testing.T) {
	neg := int64(-1)
	if err := Validate(Ticket{ID: "01", Status: StatusOpen, Type: TypeImplement, TotalTokens: &neg}); err == nil {
		t.Error("negative total_tokens passed Validate")
	}
}

func TestTicket_StampCreated(t *testing.T) {
	now := time.Date(2026, 10, 1, 1, 2, 3, 0, time.UTC)
	var tk Ticket
	tk.StampCreated(now, "gx:epic/01")
	if !tk.CreatedAt.Equal(now) || tk.OriginalAddress != "gx:epic/01" {
		t.Errorf("StampCreated = %v %q", tk.CreatedAt, tk.OriginalAddress)
	}
}
