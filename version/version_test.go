package version

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	buildInfo := func(v string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: v}}, true
		}
	}
	noBuildInfo := func() (*debug.BuildInfo, bool) { return nil, false }

	tests := []struct {
		name     string
		injected string
		read     func() (*debug.BuildInfo, bool)
		want     string
	}{
		{"ldflags wins over build info", "v1.2.3", buildInfo("v9.9.9"), "v1.2.3"},
		{"build info when no ldflags", "", buildInfo("v9.9.9"), "v9.9.9"},
		{"devel build info falls back", "", buildInfo("(devel)"), "dev"},
		{"empty build info version falls back", "", buildInfo(""), "dev"},
		{"no build info falls back", "", noBuildInfo, "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolve(tt.injected, tt.read); got != tt.want {
				t.Fatalf("resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}
