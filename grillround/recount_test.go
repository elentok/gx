package grillround_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/transcript"
)

func writeRound(t *testing.T, home, dir, id string) {
	t.Helper()
	path := transcript.PathIn(home, dir, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	line, _ := json.Marshal(map[string]any{
		"type": "assistant", "uuid": "m1", "timestamp": "2026-01-02T03:04:05Z",
		"message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": "❓ **Q1** - **Alpha**: one two"}}},
	})
	if err := os.WriteFile(path, append(line, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRecount(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	rec := conversations.Record{ID: "c1", Dir: "/work/dir", Harness: "claude", Ticket: "p:e/01"}
	if err := conversations.Save(root, rec); err != nil {
		t.Fatal(err)
	}
	writeRound(t, home, rec.Dir, rec.ID)

	if err := grillround.Recount(root, home, rec.ID, rec.Dir); err != nil {
		t.Fatal(err)
	}
	first, _ := conversations.Read(root, rec.ID)
	if len(first.Grilling) != 1 || first.Grilling[0].MessageID != "m1" || first.Grilling[0].Questions != 1 || first.Grilling[0].Words != 3 {
		t.Fatalf("counts = %+v", first.Grilling)
	}

	if err := grillround.Recount(root, home, rec.ID, rec.Dir); err != nil {
		t.Fatal(err)
	}
	if second, _ := conversations.Read(root, rec.ID); !reflect.DeepEqual(second, first) {
		t.Errorf("second run = %+v, want %+v", second, first)
	}

	if err := os.Remove(transcript.PathIn(home, rec.Dir, rec.ID)); err != nil {
		t.Fatal(err)
	}
	if err := grillround.Recount(root, home, rec.ID, rec.Dir); err != nil {
		t.Fatal(err)
	}
	if kept, _ := conversations.Read(root, rec.ID); !reflect.DeepEqual(kept, first) {
		t.Errorf("missing transcript changed the record: %+v", kept)
	}
}
