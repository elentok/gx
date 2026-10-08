package server

import (
	"errors"
	"testing"
)

func TestRetargetIfLanded(t *testing.T) {
	tests := map[string]struct {
		landed bool
		err    error
		want   string
	}{
		"unlanded blocker keeps its branch":   {want: "blocker"},
		"landed blocker retargets to default": {landed: true, want: "main"},
		"unresolvable branch is not landed":   {landed: true, err: errors.New("bad ref"), want: "blocker"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := retargetIfLanded("blocker", "main", func(string, string) (bool, error) { return tc.landed, tc.err })
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
