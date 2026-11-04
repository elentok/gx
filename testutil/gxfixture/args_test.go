package main

import (
	"reflect"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    opts
		wantErr string
	}{
		{name: "scenario only", args: []string{"ready"}, want: opts{scenario: "ready"}},
		{name: "bool flags", args: []string{"--hide-codex", "--hold", "--env", "ready"}, want: opts{scenario: "ready", hideCodex: true, hold: true, shellEnv: true}},
		{name: "value flags", args: []string{
			"--auto-advance-delay", "1s", "--command-list-age", "2s", "--stall-threshold", "3s",
			"--mock-dir", "/m", "--shape", "stale", "--gx", "/gx", "--fakeclaude", "/fc", "--fakecodex", "/fx", "mixed",
		}, want: opts{
			scenario: "mixed", autoAdvance: "1s", commandListAge: "2s", stallThreshold: "3s",
			mockDir: "/m", shape: "stale", gx: "/gx", fakeClaude: "/fc", fakeCodex: "/fx",
		}},
		{name: "repeated config", args: []string{"--config", "a.b=1", "edge", "--config", "c.d=true"}, want: opts{scenario: "edge", configs: []string{"a.b=1", "c.d=true"}}},
		{name: "last positional wins", args: []string{"ready", "mixed"}, want: opts{scenario: "mixed"}},
		{name: "unknown name is kept for the lookup", args: []string{"nope"}, want: opts{scenario: "nope"}},
		{name: "no args", args: nil, want: opts{}},
		{name: "missing value", args: []string{"ready", "--gx"}, wantErr: "--gx needs a value"},
		{name: "missing config value", args: []string{"--config"}, wantErr: "--config needs a value"},
		{name: "missing shape value", args: []string{"--shape"}, wantErr: "--shape needs a value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
