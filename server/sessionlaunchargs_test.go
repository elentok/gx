package server

import (
	"slices"
	"testing"
)

func TestLaunchArgs(t *testing.T) {
	claudeArgs := []string{"--verbose"}
	for _, tc := range []struct {
		name string
		meta sessionMeta
		want []string
	}{
		{"claude with a remote-control name", sessionMeta{Harness: "claude", RemoteControl: "proj:loop/01"}, []string{"--verbose", "--remote-control", "proj:loop/01"}},
		{"claude without a name", sessionMeta{Harness: "claude"}, []string{"--verbose"}},
		{"codex never gets it", sessionMeta{Harness: "codex", RemoteControl: "proj:loop/01"}, nil},
	} {
		if got := launchArgs(&tc.meta, claudeArgs); !slices.Equal(got, tc.want) {
			t.Errorf("%s: args = %v, want %v", tc.name, got, tc.want)
		}
	}
	if !slices.Equal(claudeArgs, []string{"--verbose"}) {
		t.Errorf("configured claude args mutated: %v", claudeArgs)
	}
}
