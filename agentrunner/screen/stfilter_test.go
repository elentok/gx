package screen

import (
	"slices"
	"testing"
)

func TestScreen_TitleWithAnSTByteInARuneDoesNotPrint(t *testing.T) {
	// ✳ is E2 9C B3; claude puts it in the window title while it works.
	seq := []byte("❯ \x1b]0;✳ GX agents\a\x1b[2mpush it\x1b[22m")
	for _, split := range []bool{false, true} {
		s := NewScreen(30, 2)
		if split {
			for i := range seq {
				s.Write(seq[i : i+1])
			}
		} else {
			s.Write(seq)
		}
		if got := s.Rows()[0]; got != "❯ push it" {
			t.Errorf("split=%v: row %q, want %q", split, got, "❯ push it")
		}
		s.Close()
	}
}

func TestScreen_ALoneSTStillEndsAString(t *testing.T) {
	s := NewScreen(30, 2)
	defer s.Close()
	s.Write([]byte("\x1b]0;title\x9cok"))
	if got := s.Rows()[0]; got != "ok" {
		t.Fatalf("row %q, want %q", got, "ok")
	}
}

func TestSTFilter_KeepsBytesOutsideStrings(t *testing.T) {
	in := []byte("✳ \x1b[1m✳\x1b]0;✳\a✳")
	got := slices.Clone(in)
	var f stFilter
	f.filter(got)
	want := slices.Clone(in)
	want[len("✳ \x1b[1m✳\x1b]0;")+1] = stSubstitute
	if !slices.Equal(got, want) {
		t.Fatalf("filter gave %q, want %q", got, want)
	}
}
