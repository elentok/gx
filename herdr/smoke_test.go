package herdr

import (
	"testing"

	"github.com/elentok/gx/testutil/herdrfake"
)

// TestFindWorkspace_ThroughFakeHerdrProcessBoundary exercises a production
// Herdr wrapper (FindWorkspace) without faking runCommand: it relies entirely
// on the fake `herdr` executable installed first in PATH by herdrfake.Start,
// proving the process boundary itself works end to end.
func TestFindWorkspace_ThroughFakeHerdrProcessBoundary(t *testing.T) {
	herdrfake.Start(t, func(argv []string) ([]byte, int) {
		if len(argv) != 2 || argv[0] != "workspace" || argv[1] != "list" {
			t.Errorf("argv = %v, want [workspace list]", argv)
			return herdrfake.CommandError("unexpected command")
		}
		return herdrfake.Result(map[string]any{
			"workspaces": []map[string]string{
				{"workspace_id": "ws-1", "label": "mylabel"},
			},
		})
	})

	id, err := FindWorkspace("mylabel")
	if err != nil {
		t.Fatalf("FindWorkspace() error = %v", err)
	}
	if id != "ws-1" {
		t.Fatalf("FindWorkspace() = %q, want %q", id, "ws-1")
	}
}
