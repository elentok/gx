package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/transcript"
)

func TestChatCache_RereadsOnlyWhenTheFileChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.jsonl")
	if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loads := 0
	load := func(string) ([]transcript.ChatMessage, error) {
		loads++
		return []transcript.ChatMessage{{ID: "m"}}, nil
	}
	var c chatCache
	for range 2 {
		if _, _, err := c.read(path, load); err != nil {
			t.Fatal(err)
		}
	}
	if loads != 1 {
		t.Fatalf("loads = %d after two reads of an unchanged file, want 1", loads)
	}
	if err := os.WriteFile(path, []byte("a\nbb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.read(path, load); err != nil {
		t.Fatal(err)
	}
	if loads != 2 {
		t.Errorf("loads = %d after the file grew, want 2", loads)
	}
}
