package skills

import (
	"reflect"
	"testing"
)

func TestSourceClashes(t *testing.T) {
	forward := map[string][]string{
		"/a": {"x", "y"},
		"/b": {"x", "z"},
		"/c": {"z", "w"},
	}
	want := map[string][]string{
		"x": {"/a", "/b"},
		"z": {"/b", "/c"},
	}
	if got := SourceClashes(forward); !reflect.DeepEqual(got, want) {
		t.Fatalf("SourceClashes = %v, want %v", got, want)
	}
}

func TestSourceClashesNoneWhenNamesDistinct(t *testing.T) {
	got := SourceClashes(map[string][]string{"/a": {"x"}, "/b": {"y"}})
	if len(got) != 0 {
		t.Fatalf("SourceClashes = %v, want none", got)
	}
}
