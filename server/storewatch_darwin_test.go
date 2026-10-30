//go:build darwin && cgo

package server

import (
	"errors"
	"testing"

	"github.com/fsnotify/fsevents"
)

func TestStoreWatchFallsBackWhenFSEventsCannotStart(t *testing.T) {
	t.Parallel()
	called := false
	testStoreWatchSeesNewFilesAndSkipsGit(t, func(root string) (*StoreWatch, error) {
		return newStoreWatch(root, func(es *fsevents.EventStream) error {
			called = true
			if len(es.Paths) != 1 || es.Paths[0] != root {
				t.Fatalf("FSEvents paths = %v, want [%s]", es.Paths, root)
			}
			return errors.New("failed to start eventstream")
		})
	})
	if !called {
		t.Fatal("FSEvents start was not attempted")
	}
}
