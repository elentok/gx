package conversations_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/elentok/gx/conversations"
)

var at = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func TestGrillingAndDesignRoundTrip(t *testing.T) {
	root := t.TempDir()
	items := []conversations.GrillingItem{{MessageID: "m1", At: at, Questions: 2, Words: 9}}
	if err := conversations.Save(root, conversations.Record{ID: "c1", Design: true, Grilling: items}); err != nil {
		t.Fatal(err)
	}
	got, err := conversations.Read(root, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Design || !reflect.DeepEqual(got.Grilling, items) {
		t.Errorf("record = %+v, want Design and %+v", got, items)
	}
}

func TestEmptyGrillingStaysCounted(t *testing.T) {
	root := t.TempDir()
	if err := conversations.Save(root, conversations.Record{ID: "c1"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := conversations.Read(root, "c1"); got.Grilling != nil {
		t.Fatalf("uncounted record has Grilling %+v, want nil", got.Grilling)
	}
	if err := conversations.SaveGrilling(root, "c1", nil); err != nil {
		t.Fatal(err)
	}
	got, _ := conversations.Read(root, "c1")
	if got.Grilling == nil || len(got.Grilling) != 0 {
		t.Errorf("Grilling = %#v, want empty non-nil", got.Grilling)
	}
}

func TestTotalsAndGrillingSavesKeepEachOther(t *testing.T) {
	root := t.TempDir()
	if err := conversations.Save(root, conversations.Record{ID: "c1", Ticket: "p:e/01"}); err != nil {
		t.Fatal(err)
	}
	items := []conversations.GrillingItem{{MessageID: "m1", At: at, Questions: 1, Words: 3}}
	if err := conversations.SaveGrilling(root, "c1", items); err != nil {
		t.Fatal(err)
	}
	if err := conversations.SaveTotals(root, "c1", conversations.Totals{TotalTokens: 7, Cost: 1.5}); err != nil {
		t.Fatal(err)
	}
	got, _ := conversations.Read(root, "c1")
	if !reflect.DeepEqual(got.Grilling, items) || got.Totals == nil || got.Totals.TotalTokens != 7 {
		t.Errorf("after totals save: %+v", got)
	}
	if err := conversations.SaveGrilling(root, "c1", nil); err != nil {
		t.Fatal(err)
	}
	if got, _ = conversations.Read(root, "c1"); got.Totals == nil || got.Totals.TotalTokens != 7 {
		t.Errorf("counts save cleared totals: %+v", got)
	}
	// A re-link save keeps both.
	if err := conversations.Save(root, conversations.Record{ID: "c1", Ticket: "p:e/02"}); err != nil {
		t.Fatal(err)
	}
	if got, _ = conversations.Read(root, "c1"); got.Totals == nil || got.Grilling == nil {
		t.Errorf("re-link save cleared fields: %+v", got)
	}
}

func TestRecordWithoutTotalsStillUndercounts(t *testing.T) {
	root := t.TempDir()
	if err := conversations.Save(root, conversations.Record{ID: "c1", Ticket: "p:e/01"}); err != nil {
		t.Fatal(err)
	}
	if err := conversations.SaveGrilling(root, "c1", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := conversations.SumRecordedTotals(root, "p:e/01"); ok {
		t.Error("counts must not make a no-totals record complete")
	}
}
