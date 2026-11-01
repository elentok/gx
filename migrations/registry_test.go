package migrations

import (
	"errors"
	"os"
	"path/filepath"
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

func appliedRegistry(ran *[]string, failB bool) *Registry {
	r := &Registry{}
	for _, n := range []string{"a", "b", "c"} {
		r.Register(Step{Name: n, Run: func(Env) ([]tickets.MigrationChange, error) {
			*ran = append(*ran, n)
			if failB && n == "b" {
				return nil, errors.New("boom")
			}
			return nil, nil
		}})
	}
	return r
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	got, err := readApplied(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRun_SkipsListedNamesAndRecordsTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), AppliedFile)
	if err := os.WriteFile(path, []byte("# note\n\na\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var ran []string
	r := appliedRegistry(&ran, false)
	if _, _, err := r.Run(Env{AppliedPath: path, SkipApplied: true}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ran, []string{"b", "c"}) {
		t.Errorf("ran %v, want [b c]", ran)
	}
	if got := readLines(t, path); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("list %v", got)
	}
	ran = nil
	if _, _, err := r.Run(Env{AppliedPath: path, SkipApplied: true}); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 0 {
		t.Errorf("second run ran %v", ran)
	}
}

func TestRun_NoListRunsEverythingThenRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), AppliedFile)
	var ran []string
	if _, _, err := appliedRegistry(&ran, false).Run(Env{AppliedPath: path, SkipApplied: true}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ran, []string{"a", "b", "c"}) || !slices.Equal(readLines(t, path), ran) {
		t.Errorf("ran %v, list %v", ran, readLines(t, path))
	}
}

func TestRun_FailedStepIsNotRecordedAndLaterRunResumesThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), AppliedFile)
	var ran []string
	if _, _, err := appliedRegistry(&ran, true).Run(Env{AppliedPath: path, SkipApplied: true}); err == nil {
		t.Fatal("want an error")
	}
	if !slices.Equal(ran, []string{"a", "b"}) || !slices.Equal(readLines(t, path), []string{"a"}) {
		t.Errorf("ran %v, list %v", ran, readLines(t, path))
	}
	ran = nil
	if _, _, err := appliedRegistry(&ran, false).Run(Env{AppliedPath: path, SkipApplied: true}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ran, []string{"b", "c"}) {
		t.Errorf("resumed run ran %v", ran)
	}
}

func TestRun_ManualRunSkipsNothingAndAddsNoDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), AppliedFile)
	if err := os.WriteFile(path, []byte("a\nb\nc"), 0o644); err != nil {
		t.Fatal(err)
	}
	var ran []string
	if _, _, err := appliedRegistry(&ran, false).Run(Env{AppliedPath: path}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ran, []string{"a", "b", "c"}) || !slices.Equal(readLines(t, path), ran) {
		t.Errorf("ran %v, list %v", ran, readLines(t, path))
	}
}

// Two steps X and Y that land in either order: a store holding only one of them
// runs only the other, and both lists end holding both.
func TestRun_StepsLandingInEitherOrderEachRunOnlyTheMissingOne(t *testing.T) {
	r := &Registry{}
	var ran []string
	for _, n := range []string{"x", "y"} {
		r.Register(Step{Name: n, Run: func(Env) ([]tickets.MigrationChange, error) {
			ran = append(ran, n)
			return nil, nil
		}})
	}
	for _, listed := range []string{"x", "y"} {
		path := filepath.Join(t.TempDir(), AppliedFile)
		if err := os.WriteFile(path, []byte(listed+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ran = nil
		if _, _, err := r.Run(Env{AppliedPath: path, SkipApplied: true}); err != nil {
			t.Fatal(err)
		}
		missing := map[string]string{"x": "y", "y": "x"}[listed]
		if !slices.Equal(ran, []string{missing}) {
			t.Errorf("store listing %s: ran %v, want [%s]", listed, ran, missing)
		}
		got := slices.Clone(readLines(t, path))
		slices.Sort(got)
		if !slices.Equal(got, []string{"x", "y"}) {
			t.Errorf("store listing %s: list %v, want x and y", listed, got)
		}
	}
}

func TestRun_NoAppliedPathWritesNoList(t *testing.T) {
	dir := t.TempDir()
	var ran []string
	if _, _, err := appliedRegistry(&ran, false).Run(Env{Root: dir}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("dir has %v", entries)
	}
}

func TestUnknownApplied_OneSortedSetAcrossProjects(t *testing.T) {
	var ran []string
	r := appliedRegistry(&ran, false)
	d1, d2 := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(d1, AppliedFile), []byte("a\nz-new\nm-new\n"), 0o644)
	os.WriteFile(filepath.Join(d2, AppliedFile), []byte("z-new\nb\n"), 0o644)
	got, err := r.UnknownApplied(d1, d2, t.TempDir())
	if err != nil || !slices.Equal(got, []string{"m-new", "z-new"}) {
		t.Errorf("got %v, err %v", got, err)
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
