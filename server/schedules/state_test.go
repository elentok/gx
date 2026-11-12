package schedules_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/schedules"
)

var (
	key = schedules.Key{Project: "gx", Slug: "nightly"}
	t0  = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
)

func open(t *testing.T, dir string) *schedules.State {
	t.Helper()
	s, err := schedules.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMachineID_StableAcrossReopenAndDistinctPerDir(t *testing.T) {
	dir := t.TempDir()
	id := open(t, dir).MachineID()
	if id == "" || open(t, dir).MachineID() != id {
		t.Fatalf("id not stable: %q", id)
	}
	if open(t, t.TempDir()).MachineID() == id {
		t.Fatal("two state dirs share an id")
	}
}

func TestOpenWithID_UsedOnlyWhenNoneStored(t *testing.T) {
	dir := t.TempDir()
	s, err := schedules.OpenWithID(dir, "fixed")
	if err != nil || s.MachineID() != "fixed" {
		t.Fatalf("got %v, %v", s, err)
	}
	s2, err := schedules.OpenWithID(dir, "other")
	if err != nil || s2.MachineID() != "fixed" {
		t.Fatalf("stored id lost: %v, %v", s2, err)
	}
}

func TestSwitch_DefaultsOffEnableSetsBaselineDisableKeepsIt(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	if s.Enabled(key) {
		t.Fatal("default must be off")
	}
	if _, ok := s.Baseline(key); ok {
		t.Fatal("default has no baseline")
	}
	if err := s.Enable(key, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Disable(key); err != nil {
		t.Fatal(err)
	}
	s = open(t, dir)
	if s.Enabled(key) {
		t.Fatal("still on after disable")
	}
	if b, ok := s.Baseline(key); !ok || !b.Equal(t0) {
		t.Fatalf("baseline = %v, %v", b, ok)
	}
	if err := s.Enable(key, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.Baseline(key); !b.Equal(t0.Add(time.Hour)) {
		t.Fatalf("enable did not reset baseline: %v", b)
	}
}

func TestAdvance_NeverGoesBackwards(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	for _, at := range []time.Time{t0, t0.Add(-time.Hour), t0.Add(time.Hour), t0} {
		if err := s.Advance(key, at); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := open(t, dir).Baseline(key); !b.Equal(t0.Add(time.Hour)) {
		t.Fatalf("baseline = %v", b)
	}
}

func TestSkips_KeepNewestTenInOrderAndSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	for i := range 12 {
		err := s.AppendSkip(key, schedules.Skip{
			SetTime: t0.Add(time.Duration(i) * time.Hour), Reason: schedules.ReasonLate, LoggedAt: t0,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	got := open(t, dir).Skips(key)
	if len(got) != 10 || !got[0].SetTime.Equal(t0.Add(2*time.Hour)) || !got[9].SetTime.Equal(t0.Add(11*time.Hour)) {
		t.Fatalf("got %d skips, first %v last %v", len(got), got[0].SetTime, got[len(got)-1].SetTime)
	}
}

func TestSkips_KeepBlockingAddressAndLogged(t *testing.T) {
	s := open(t, t.TempDir())
	sk := schedules.Skip{SetTime: t0, Reason: schedules.ReasonPreviousRunNeedsAttention, LoggedAt: t0, Blocking: "gx:epic/01"}
	if err := s.AppendSkip(key, sk); err != nil {
		t.Fatal(err)
	}
	if got := s.Skips(key); len(got) != 1 || got[0].Blocking != "gx:epic/01" {
		t.Fatalf("got %+v", got)
	}
	if !s.Logged(key, t0, schedules.ReasonPreviousRunNeedsAttention) {
		t.Fatal("should be logged")
	}
	if s.Logged(key, t0, schedules.ReasonLate) || s.Logged(key, t0.Add(time.Minute), schedules.ReasonPreviousRunNeedsAttention) {
		t.Fatal("wrong reason or time reported as logged")
	}
}

func TestDeleteAndKeys_CleanupOneSchedule(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	other := schedules.Key{Project: "gx", Slug: "weekly"}
	for _, k := range []schedules.Key{key, other} {
		if err := s.Enable(k, t0); err != nil {
			t.Fatal(err)
		}
		if err := s.AppendSkip(k, schedules.Skip{SetTime: t0, Reason: schedules.ReasonLate}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete(key); err != nil {
		t.Fatal(err)
	}
	s = open(t, dir)
	if s.Enabled(key) || len(s.Skips(key)) != 0 {
		t.Fatal("entries of deleted Schedule remain")
	}
	if _, ok := s.Baseline(key); ok {
		t.Fatal("baseline remains")
	}
	if got := s.Keys(); !slices.Equal(got, []schedules.Key{other}) {
		t.Fatal(fmt.Sprint("keys = ", got))
	}
	if !s.Enabled(other) {
		t.Fatal("other Schedule lost")
	}
}
