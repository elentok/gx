package server

import (
	"errors"
	"testing"
)

func TestRetargetIfMerged(t *testing.T) {
	tests := map[string]struct {
		merged bool
		err    error
		want   string
	}{
		"unlanded blocker keeps its branch": {want: "blocker"},
		"landed blocker retargets to trunk": {merged: true, want: "main"},
		"unresolvable branch is not landed": {merged: true, err: errors.New("bad ref"), want: "blocker"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := retargetIfMerged("blocker", "main", func(string, string) (bool, error) { return tc.merged, tc.err })
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
