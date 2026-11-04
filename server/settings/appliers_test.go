package settings

import (
	"slices"
	"testing"
)

func TestAppliers_RunOrder(t *testing.T) {
	var calls []string
	rec := func(name string, closer bool) Applier {
		return func(old, next Settings) func() {
			calls = append(calls, name)
			if !closer {
				return nil
			}
			return func() { calls = append(calls, "close-"+name) }
		}
	}
	a := Appliers{
		Before: []Applier{rec("b1", true), rec("b2", false)},
		After:  []Applier{rec("a1", true), rec("a2", false)},
	}
	closers := a.run(Settings{}, Settings{}, func() { calls = append(calls, "swap") })
	want := []string{"b1", "b2", "swap", "a1", "a2"}
	if !slices.Equal(calls, want) {
		t.Fatalf("apply order = %v, want %v", calls, want)
	}
	joinClosers(closers)()
	want = append(want, "close-b1", "close-a1")
	if !slices.Equal(calls, want) {
		t.Fatalf("with closers = %v, want %v", calls, want)
	}
	if joinClosers(nil) != nil {
		t.Fatal("no closers should join to nil")
	}
}
