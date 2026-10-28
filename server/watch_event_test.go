package server

import (
	"testing"

	"github.com/fsnotify/fsnotify"
)

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
