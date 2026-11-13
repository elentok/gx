package harness

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
)

func TestWritableDirectoryArgs(t *testing.T) {
	t.Parallel()
	for _, kind := range []agentrunner.Kind{Claude, Codex} {
		t.Run(string(kind), func(t *testing.T) {
			var args []string
			if kind == Codex {
				args = CodexArgs(CodexArgsInput{WritableRoots: []string{"/tickets"}})
			}
			original := slices.Clone(args)
			got := WritableDirectoryArgs(kind, args, []string{"~/Library/pnpm", "cache", "~/Library/pnpm"}, "/isolated/home", "/repo")
			joined := strings.Join(got, " ")
			for _, path := range []string{"/isolated/home/Library/pnpm", "/repo/cache"} {
				if strings.Count(joined, path) != 1 {
					t.Fatalf("expected one %s in %v", path, got)
				}
			}
			if !slices.Equal(args, original) {
				t.Fatal("mutated caller arguments")
			}
			if kind == Codex && (!strings.Contains(joined, "/tickets") || strings.Count(joined, "writable_roots=") != 1) {
				t.Fatalf("existing roots not preserved: %v", got)
			}
			if got := WritableDirectoryArgs(kind, args, nil, "/home", "/repo"); !slices.Equal(got, args) {
				t.Fatal("empty list changed arguments")
			}
		})
	}
}
