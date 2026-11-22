package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/broker"
	"github.com/fsnotify/fsnotify"
)

func TestRescan_MovesTheGenerationAndRefreshesTheProjectCache(t *testing.T) {
	store := t.TempDir()
	write := func(name string) {
		dir := filepath.Join(store, "p")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(`{"name":"`+name+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("one")
	idx := New(store, broker.New(1), nil)
	cache := idx.ProjectCache()
	dirs, err := cache.Dirs(store)
	if err != nil || len(dirs) != 1 {
		t.Fatalf("dirs = %v, %v", dirs, err)
	}
	if got := cache.Name(dirs[0]); got != "one" {
		t.Fatalf("name = %q, want one", got)
	}
	write("two")
	before := idx.Generation()
	idx.Rescan()
	if idx.Generation() == before {
		t.Fatal("generation did not move on a rescan")
	}
	if _, err := cache.Dirs(store); err != nil {
		t.Fatal(err)
	}
	if got := cache.Name(dirs[0]); got != "two" {
		t.Fatalf("name after a rescan = %q, want two", got)
	}
}

func TestStoreEventKind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tests := []struct {
		name         string
		ev           fsnotify.Event
		wantRelevant bool
		wantNewDir   bool
	}{
		{"run-log append", fsnotify.Event{Name: "/s/p/e/run-log.jsonl", Op: fsnotify.Write}, false, false},
		{"run-log create", fsnotify.Event{Name: "/s/p/e/run-log.jsonl", Op: fsnotify.Create}, false, false},
		{"ticket write", fsnotify.Event{Name: "/s/p/e/issues/01-x.md", Op: fsnotify.Write}, true, false},
		{"atomic temp rename", fsnotify.Event{Name: "/s/p/e/issues/01-x.md.tmp-123", Op: fsnotify.Rename}, true, false},
		{"directory created", fsnotify.Event{Name: dir, Op: fsnotify.Create}, true, true},
		{"directory removed", fsnotify.Event{Name: "/s/p/gone", Op: fsnotify.Remove}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relevant, newDir := storeEventKind(tt.ev)
			if relevant != tt.wantRelevant || newDir != tt.wantNewDir {
				t.Errorf("storeEventKind = (%v, %v), want (%v, %v)", relevant, newDir, tt.wantRelevant, tt.wantNewDir)
			}
		})
	}
}
