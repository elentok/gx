package ui

import (
	"strings"
	"testing"
)

func TestEditorLaunchArgsUsesGotoForKnownEditors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		editor string
		want   string
	}{
		{name: "code", editor: "code", want: "--goto /tmp/x.go:12"},
		{name: "vim", editor: "nvim", want: "+12 /tmp/x.go"},
		{name: "sublime", editor: "subl", want: "/tmp/x.go:12"},
		{name: "fallback", editor: "emacs", want: "/tmp/x.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(EditorLaunchArgs(tt.editor, nil, "/tmp/x.go", 12), " ")
			if !strings.Contains(got, tt.want) {
				t.Fatalf("EditorLaunchArgs(%q)=%q, want to contain %q", tt.editor, got, tt.want)
			}
		})
	}
}
