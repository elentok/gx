package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/harness"
)

func TestChatCache_RereadsOnlyWhenTheFileChanged(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "c.jsonl")
	if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loads := 0
	load := func(string) (chatSnapshot, error) {
		loads++
		return chatSnapshot{msgs: []SessionChatMessage{{ID: "m"}}}, nil
	}
	var c chatCache
	for range 2 {
		if _, err := c.read(path, harness.Claude, false, load); err != nil {
			t.Fatal(err)
		}
	}
	if loads != 1 {
		t.Fatalf("loads = %d after two reads of an unchanged file, want 1", loads)
	}
	if err := os.WriteFile(path, []byte("a\nbb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.read(path, harness.Claude, false, load); err != nil {
		t.Fatal(err)
	}
	if loads != 2 {
		t.Errorf("loads = %d after the file grew, want 2", loads)
	}
	if _, err := c.read(path, harness.Claude, true, load); err != nil {
		t.Fatal(err)
	}
	if loads != 3 {
		t.Errorf("loads = %d after the final flag flipped, want 3", loads)
	}
	if _, err := c.read(path, harness.Codex, true, load); err != nil {
		t.Fatal(err)
	}
	if loads != 4 {
		t.Errorf("loads = %d after the harness changed, want 4", loads)
	}
}
