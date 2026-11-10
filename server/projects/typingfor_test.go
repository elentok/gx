package projects_test

import (
	"testing"

	"github.com/elentok/gx/server/projects"
)

func TestTypingFor_UnknownAndEmptyKindsInterruptWithCtrlC(t *testing.T) {
	reg := projects.NewRegistry(func() projects.Env { return projects.Env{} }, nil, nil, nil, nil, nil)
	for _, kind := range []string{"", "nochat", "claude"} {
		if got := reg.TypingFor(kind).InterruptKey(); got != "\x03" {
			t.Errorf("TypingFor(%q).InterruptKey = %q, want Ctrl+C", kind, got)
		}
	}
}
