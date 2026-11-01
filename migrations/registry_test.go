package migrations

import (
	"errors"
	"slices"
	"testing"

	"github.com/elentok/gx/tickets"
)

func noop(Env) ([]tickets.MigrationChange, error) { return nil, nil }

func mustPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected a panic")
		}
	}()
	f()
}

func TestRegister_DuplicateNamePanics(t *testing.T) {
	r := &Registry{}
	r.Register(Step{Name: "a", Run: noop})
	mustPanic(t, func() { r.Register(Step{Name: "a", Run: noop}) })
}

func TestRegister_EmptyNamePanics(t *testing.T) {
	mustPanic(t, func() { (&Registry{}).Register(Step{Run: noop}) })
}

func TestRun_KeepsRegistrationOrder(t *testing.T) {
	r := &Registry{}
	var ran []string
	for _, n := range []string{"c", "a", "b"} {
		r.Register(Step{Name: n, Run: func(Env) ([]tickets.MigrationChange, error) {
			ran = append(ran, n)
			return nil, nil
		}})
	}
	if _, _, err := r.Run(Env{}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"c", "a", "b"}; !slices.Equal(ran, want) || !slices.Equal(r.Names(), want) {
		t.Errorf("ran %v, names %v, want %v", ran, r.Names(), want)
	}
}

func TestRun_SkipsByName(t *testing.T) {
	r := &Registry{}
	var ran []string
	for _, n := range []string{"a", "b"} {
		r.Register(Step{Name: n, Run: func(Env) ([]tickets.MigrationChange, error) {
			ran = append(ran, n)
			return nil, nil
		}})
	}
	if _, _, err := r.Run(Env{Skip: func(n string) bool { return n == "a" }}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ran, []string{"b"}) {
		t.Errorf("ran %v, want [b]", ran)
	}
}

func TestRun_StopsAtFirstFailureKeepingChanges(t *testing.T) {
	r := &Registry{}
	boom := errors.New("boom")
	r.Register(Step{Name: "a", Run: func(Env) ([]tickets.MigrationChange, error) {
		return []tickets.MigrationChange{{Path: "x", Notes: []string{"1"}}}, boom
	}})
	r.Register(Step{Name: "b", Run: func(Env) ([]tickets.MigrationChange, error) {
		t.Error("step after a failure ran")
		return nil, nil
	}})
	changes, early, err := r.Run(Env{})
	if !errors.Is(err, boom) || early || len(changes) != 1 {
		t.Errorf("changes %v, early %v, err %v", changes, early, err)
	}
}

func TestRun_MergesSameFileUnlessAppend(t *testing.T) {
	r := &Registry{}
	one := func(note string) StepFunc {
		return func(Env) ([]tickets.MigrationChange, error) {
			return []tickets.MigrationChange{{Path: "x", Notes: []string{note}}}, nil
		}
	}
	r.Register(Step{Name: "a", Run: one("a")})
	r.Register(Step{Name: "b", Run: one("b")})
	r.Register(Step{Name: "c", Append: true, Run: one("c")})
	changes, _, err := r.Run(Env{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || !slices.Equal(changes[0].Notes, []string{"a", "b"}) {
		t.Errorf("changes %v", changes)
	}
}
