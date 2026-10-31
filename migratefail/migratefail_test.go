package migratefail_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/elentok/gx/migratefail"
)

func TestRecord_SurvivesAndReadsSorted(t *testing.T) {
	dir := t.TempDir()
	if err := migratefail.Record(dir, "b", "boom b"); err != nil {
		t.Fatal(err)
	}
	if err := migratefail.Record(dir, "a", "boom a"); err != nil {
		t.Fatal(err)
	}
	if err := migratefail.Record(dir, "b", "newer b"); err != nil {
		t.Fatal(err)
	}
	got, err := migratefail.Read(dir, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	want := []migratefail.Failure{{Project: "a", Error: "boom a"}, {Project: "b", Error: "newer b"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestClear_RemovesOneProject(t *testing.T) {
	dir := t.TempDir()
	_ = migratefail.Record(dir, "a", "x")
	_ = migratefail.Record(dir, "b", "y")
	if err := migratefail.Clear(dir, "a"); err != nil {
		t.Fatal(err)
	}
	got, _ := migratefail.Read(dir, []string{"a", "b"})
	if len(got) != 1 || got[0].Project != "b" {
		t.Fatalf("got %v", got)
	}
}

func TestClear_NoEntryWritesNothing(t *testing.T) {
	dir := t.TempDir()
	if err := migratefail.Clear(dir, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(migratefail.Path(dir)); !os.IsNotExist(err) {
		t.Fatalf("file created: %v", err)
	}
}

func TestRead_DropsUnconfiguredProjects(t *testing.T) {
	dir := t.TempDir()
	_ = migratefail.Record(dir, "gone", "x")
	_ = migratefail.Record(dir, "kept", "y")
	got, err := migratefail.Read(dir, []string{"kept"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Project != "kept" {
		t.Fatalf("got %v", got)
	}
}
