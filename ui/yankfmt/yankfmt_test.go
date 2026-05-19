package yankfmt

import (
	"strings"
	"testing"
)

func TestFormatYankLocation(t *testing.T) {
	tests := []struct {
		path, loc string
		want      string
	}{
		{"", "", ""},
		{"", "L5", ""},
		{"foo/bar.go", "", "@foo/bar.go"},
		{"foo/bar.go", "L5-10", "@foo/bar.go L5-10"},
		{"  foo/bar.go  ", "  L5  ", "@foo/bar.go L5"},
	}
	for _, tt := range tests {
		got := FormatYankLocation(tt.path, tt.loc)
		if got != tt.want {
			t.Errorf("FormatYankLocation(%q, %q) = %q, want %q", tt.path, tt.loc, got, tt.want)
		}
	}
}

func TestFormatYankAllContext(t *testing.T) {
	body := []string{"line1", "line2"}
	got := FormatYankAllContext("foo.go", "L1", body)
	if !strings.HasPrefix(got, "@foo.go L1\n\n") {
		t.Errorf("unexpected prefix: %q", got)
	}
	if !strings.Contains(got, "line1\nline2") {
		t.Errorf("body not found in: %q", got)
	}
}

func TestFormatForAgent(t *testing.T) {
	body := []string{"+added", "-removed"}
	got := FormatForAgent("foo.go", "L3", body)
	if !strings.Contains(got, "```diff\n+added\n-removed\n```") {
		t.Errorf("unexpected output: %q", got)
	}
	if !strings.HasPrefix(got, "@foo.go L3\n") {
		t.Errorf("unexpected prefix: %q", got)
	}
}
